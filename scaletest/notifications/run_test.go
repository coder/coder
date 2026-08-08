package notifications_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	notificationsLib "github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/coderd/notifications/types"
	"github.com/coder/coder/v2/scaletest/notifications"
	"github.com/coder/coder/v2/scaletest/smtpmock"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRun(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := testutil.Logger(t)
	db, ps := dbtestutil.NewDB(t)

	inboxHandler := dispatch.NewInboxHandler(logger.Named("inbox"), db, ps)

	client := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   ps,
	})
	firstUser := coderdtest.CreateFirstUser(t, client)

	const numReceivingUsers = 2
	const numRegularUsers = 2
	const totalUsers = numReceivingUsers + numRegularUsers
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	// The generator triggers a single template-deleted notification, so that is
	// the notification the receiving runners expect.
	expectedNotificationsIDs := map[uuid.UUID]struct{}{
		notificationsLib.TemplateTemplateDeleted: {},
	}

	dialBarrier := new(sync.WaitGroup)
	receivingWatchBarrier := new(sync.WaitGroup)
	dialBarrier.Add(totalUsers)
	receivingWatchBarrier.Add(numReceivingUsers)

	eg, runCtx := errgroup.WithContext(ctx)

	receivingRunners := make([]*notifications.Runner, 0, numReceivingUsers)
	receivingUsernames := make([]string, 0, numReceivingUsers)
	for i := 0; i < numReceivingUsers; i++ {
		userClient, user := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		receivingUsernames = append(receivingUsernames, user.Username)
		runnerCfg := notifications.Config{
			PreCreatedUser:           user,
			SessionToken:             userClient.SessionToken(),
			NotificationTimeout:      testutil.WaitLong,
			DialTimeout:              testutil.WaitLong,
			Metrics:                  metrics,
			DialBarrier:              dialBarrier,
			ReceivingWatchBarrier:    receivingWatchBarrier,
			ExpectedNotificationsIDs: expectedNotificationsIDs,
		}
		require.NoError(t, runnerCfg.Validate())

		runner := notifications.NewRunner(client, runnerCfg)
		receivingRunners = append(receivingRunners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, "receiving-"+strconv.Itoa(i), io.Discard)
		})
	}

	for i := 0; i < numRegularUsers; i++ {
		userClient, user := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		runnerCfg := notifications.Config{
			PreCreatedUser:        user,
			SessionToken:          userClient.SessionToken(),
			NotificationTimeout:   testutil.WaitLong,
			DialTimeout:           testutil.WaitLong,
			Metrics:               metrics,
			DialBarrier:           dialBarrier,
			ReceivingWatchBarrier: receivingWatchBarrier,
		}
		require.NoError(t, runnerCfg.Validate())

		runner := notifications.NewRunner(client, runnerCfg)
		eg.Go(func() error {
			return runner.Run(runCtx, "regular-"+strconv.Itoa(i), io.Discard)
		})
	}

	// Once all runners are connected, dispatch the notification to each
	// receiving user.
	eg.Go(func() error {
		dialBarrier.Wait()

		for _, username := range receivingUsernames {
			err := sendInboxNotification(runCtx, t, db, inboxHandler, username, notificationsLib.TemplateTemplateDeleted)
			require.NoError(t, err)
		}

		return nil
	})

	err := eg.Wait()
	require.NoError(t, err, "runner execution should complete successfully")

	for _, runner := range receivingRunners {
		metrics := runner.GetMetrics()
		websocketReceiptTimes := metrics[notifications.WebsocketNotificationReceiptTimeMetric].(map[uuid.UUID]time.Time)

		require.Contains(t, websocketReceiptTimes, notificationsLib.TemplateTemplateDeleted)
	}
}

func TestRunWithSMTP(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := testutil.Logger(t)
	db, ps := dbtestutil.NewDB(t)

	inboxHandler := dispatch.NewInboxHandler(logger.Named("inbox"), db, ps)

	client := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   ps,
	})
	firstUser := coderdtest.CreateFirstUser(t, client)

	smtpAPIMux := http.NewServeMux()
	smtpAPIMux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		summaries := []smtpmock.EmailSummary{
			{
				Subject:                "TemplateTemplateDeleted",
				Date:                   time.Now(),
				NotificationTemplateID: notificationsLib.TemplateTemplateDeleted,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(summaries)
	})

	smtpAPIServer := httptest.NewServer(smtpAPIMux)
	defer smtpAPIServer.Close()

	const numReceivingUsers = 2
	const numRegularUsers = 2
	const totalUsers = numReceivingUsers + numRegularUsers
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	expectedNotificationsIDs := map[uuid.UUID]struct{}{
		notificationsLib.TemplateTemplateDeleted: {},
	}

	mClock := quartz.NewMock(t)
	smtpTrap := mClock.Trap().TickerFunc("smtp")
	defer smtpTrap.Close()

	httpClient := &http.Client{}

	dialBarrier := new(sync.WaitGroup)
	receivingWatchBarrier := new(sync.WaitGroup)
	dialBarrier.Add(totalUsers)
	receivingWatchBarrier.Add(numReceivingUsers)

	eg, runCtx := errgroup.WithContext(ctx)

	receivingRunners := make([]*notifications.Runner, 0, numReceivingUsers)
	receivingUsernames := make([]string, 0, numReceivingUsers)
	for i := 0; i < numReceivingUsers; i++ {
		userClient, user := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		receivingUsernames = append(receivingUsernames, user.Username)
		runnerCfg := notifications.Config{
			PreCreatedUser:           user,
			SessionToken:             userClient.SessionToken(),
			NotificationTimeout:      testutil.WaitLong,
			DialTimeout:              testutil.WaitLong,
			Metrics:                  metrics,
			DialBarrier:              dialBarrier,
			ReceivingWatchBarrier:    receivingWatchBarrier,
			ExpectedNotificationsIDs: expectedNotificationsIDs,
			SMTPApiURL:               smtpAPIServer.URL,
			SMTPRequestTimeout:       testutil.WaitLong,
			SMTPHttpClient:           httpClient,
		}
		require.NoError(t, runnerCfg.Validate())

		runner := notifications.NewRunner(client, runnerCfg).WithClock(mClock)
		receivingRunners = append(receivingRunners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, "receiving-"+strconv.Itoa(i), io.Discard)
		})
	}

	for i := 0; i < numRegularUsers; i++ {
		userClient, user := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		runnerCfg := notifications.Config{
			PreCreatedUser:        user,
			SessionToken:          userClient.SessionToken(),
			NotificationTimeout:   testutil.WaitLong,
			DialTimeout:           testutil.WaitLong,
			Metrics:               metrics,
			DialBarrier:           dialBarrier,
			ReceivingWatchBarrier: receivingWatchBarrier,
		}
		require.NoError(t, runnerCfg.Validate())

		runner := notifications.NewRunner(client, runnerCfg)
		eg.Go(func() error {
			return runner.Run(runCtx, "regular-"+strconv.Itoa(i), io.Discard)
		})
	}

	eg.Go(func() error {
		dialBarrier.Wait()

		for range receivingUsernames {
			smtpTrap.MustWait(runCtx).MustRelease(runCtx)
		}

		for _, username := range receivingUsernames {
			err := sendInboxNotification(runCtx, t, db, inboxHandler, username, notificationsLib.TemplateTemplateDeleted)
			require.NoError(t, err)
		}

		_, w := mClock.AdvanceNext()
		w.MustWait(runCtx)

		return nil
	})

	err := eg.Wait()
	require.NoError(t, err, "runner execution with SMTP should complete successfully")

	// Verify that notifications were received via both websocket and SMTP.
	for _, runner := range receivingRunners {
		metrics := runner.GetMetrics()
		websocketReceiptTimes := metrics[notifications.WebsocketNotificationReceiptTimeMetric].(map[uuid.UUID]time.Time)
		smtpReceiptTimes := metrics[notifications.SMTPNotificationReceiptTimeMetric].(map[uuid.UUID]time.Time)

		require.Contains(t, websocketReceiptTimes, notificationsLib.TemplateTemplateDeleted)
		require.Contains(t, smtpReceiptTimes, notificationsLib.TemplateTemplateDeleted)
	}
}

func sendInboxNotification(ctx context.Context, t *testing.T, db database.Store, inboxHandler *dispatch.InboxHandler, username string, templateID uuid.UUID) error {
	user, err := db.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
		Username: username,
	})
	require.NoError(t, err)

	dispatchFunc, err := inboxHandler.Dispatcher(types.MessagePayload{
		UserID:                 user.ID.String(),
		NotificationTemplateID: templateID.String(),
	}, "", "", nil)
	if err != nil {
		return err
	}

	_, err = dispatchFunc(ctx, uuid.New())
	if err != nil {
		return err
	}

	return nil
}
