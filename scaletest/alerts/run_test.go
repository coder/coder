package alerts_test

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

	notificationsLib "github.com/coder/coder/v2/coderd/alerts"
	"github.com/coder/coder/v2/coderd/alerts/dispatch"
	"github.com/coder/coder/v2/coderd/alerts/types"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/alerts"
	"github.com/coder/coder/v2/scaletest/createusers"
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
	dialBarrier := new(sync.WaitGroup)
	receivingWatchBarrier := new(sync.WaitGroup)
	dialBarrier.Add(numReceivingUsers + numRegularUsers)
	receivingWatchBarrier.Add(numReceivingUsers)
	metrics := alerts.NewMetrics(prometheus.NewRegistry())

	eg, runCtx := errgroup.WithContext(ctx)

	expectedNotificationsIDs := map[uuid.UUID]struct{}{
		notificationsLib.TemplateUserAccountCreated: {},
		notificationsLib.TemplateUserAccountDeleted: {},
	}

	// Start receiving runners who will receive notifications
	receivingRunners := make([]*alerts.Runner, 0, numReceivingUsers)
	for i := range numReceivingUsers {
		runnerCfg := alerts.Config{
			User: createusers.Config{
				OrganizationID: firstUser.OrganizationID,
				Username:       "receiving-user-" + strconv.Itoa(i),
			},
			Roles:                    []string{codersdk.RoleOwner},
			NotificationTimeout:      testutil.WaitLong,
			DialTimeout:              testutil.WaitLong,
			Metrics:                  metrics,
			DialBarrier:              dialBarrier,
			ReceivingWatchBarrier:    receivingWatchBarrier,
			ExpectedNotificationsIDs: expectedNotificationsIDs,
		}
		err := runnerCfg.Validate()
		require.NoError(t, err)

		runner := alerts.NewRunner(client, runnerCfg)
		receivingRunners = append(receivingRunners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, "receiving-"+strconv.Itoa(i), io.Discard)
		})
	}

	// Start regular user runners who will maintain websocket connections
	regularRunners := make([]*alerts.Runner, 0, numRegularUsers)
	for i := range numRegularUsers {
		runnerCfg := alerts.Config{
			User: createusers.Config{
				OrganizationID: firstUser.OrganizationID,
			},
			Roles:                 []string{},
			NotificationTimeout:   testutil.WaitLong,
			DialTimeout:           testutil.WaitLong,
			Metrics:               metrics,
			DialBarrier:           dialBarrier,
			ReceivingWatchBarrier: receivingWatchBarrier,
		}
		err := runnerCfg.Validate()
		require.NoError(t, err)

		runner := alerts.NewRunner(client, runnerCfg)
		regularRunners = append(regularRunners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, "regular-"+strconv.Itoa(i), io.Discard)
		})
	}

	// Trigger notifications by creating and deleting a user
	eg.Go(func() error {
		// Wait for all runners to connect
		dialBarrier.Wait()

		for i := 0; i < numReceivingUsers; i++ {
			err := sendInboxAlert(runCtx, t, db, inboxHandler, "receiving-user-"+strconv.Itoa(i), notificationsLib.TemplateUserAccountCreated)
			require.NoError(t, err)
			err = sendInboxAlert(runCtx, t, db, inboxHandler, "receiving-user-"+strconv.Itoa(i), notificationsLib.TemplateUserAccountDeleted)
			require.NoError(t, err)
		}

		return nil
	})

	err := eg.Wait()
	require.NoError(t, err, "runner execution should complete successfully")

	cleanupEg, cleanupCtx := errgroup.WithContext(ctx)
	for i, runner := range receivingRunners {
		cleanupEg.Go(func() error {
			return runner.Cleanup(cleanupCtx, "receiving-"+strconv.Itoa(i), io.Discard)
		})
	}
	for i, runner := range regularRunners {
		cleanupEg.Go(func() error {
			return runner.Cleanup(cleanupCtx, "regular-"+strconv.Itoa(i), io.Discard)
		})
	}
	err = cleanupEg.Wait()
	require.NoError(t, err)

	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 1)
	require.Equal(t, firstUser.UserID, users.Users[0].ID)

	for _, runner := range receivingRunners {
		metrics := runner.GetMetrics()
		websocketReceiptTimes := metrics[alerts.WebsocketNotificationReceiptTimeMetric].(map[uuid.UUID]time.Time)

		require.Contains(t, websocketReceiptTimes, notificationsLib.TemplateUserAccountCreated)
		require.Contains(t, websocketReceiptTimes, notificationsLib.TemplateUserAccountDeleted)
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
				Subject:         "TemplateUserAccountCreated",
				Date:            time.Now(),
				AlertTemplateID: notificationsLib.TemplateUserAccountCreated,
			},
			{
				Subject:         "TemplateUserAccountDeleted",
				Date:            time.Now(),
				AlertTemplateID: notificationsLib.TemplateUserAccountDeleted,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(summaries)
	})

	smtpAPIServer := httptest.NewServer(smtpAPIMux)
	defer smtpAPIServer.Close()

	const numReceivingUsers = 2
	const numRegularUsers = 2
	dialBarrier := new(sync.WaitGroup)
	receivingWatchBarrier := new(sync.WaitGroup)
	dialBarrier.Add(numReceivingUsers + numRegularUsers)
	receivingWatchBarrier.Add(numReceivingUsers)
	metrics := alerts.NewMetrics(prometheus.NewRegistry())

	eg, runCtx := errgroup.WithContext(ctx)

	expectedNotificationsIDs := map[uuid.UUID]struct{}{
		notificationsLib.TemplateUserAccountCreated: {},
		notificationsLib.TemplateUserAccountDeleted: {},
	}

	mClock := quartz.NewMock(t)
	smtpTrap := mClock.Trap().TickerFunc("smtp")
	defer smtpTrap.Close()

	httpClient := &http.Client{}

	// Start receiving runners who will receive notifications
	receivingRunners := make([]*alerts.Runner, 0, numReceivingUsers)
	for i := range numReceivingUsers {
		runnerCfg := alerts.Config{
			User: createusers.Config{
				OrganizationID: firstUser.OrganizationID,
				Username:       "receiving-user-" + strconv.Itoa(i),
			},
			Roles:                    []string{codersdk.RoleOwner},
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
		err := runnerCfg.Validate()
		require.NoError(t, err)

		runner := alerts.NewRunner(client, runnerCfg).WithClock(mClock)
		receivingRunners = append(receivingRunners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, "receiving-"+strconv.Itoa(i), io.Discard)
		})
	}

	// Start regular user runners who will maintain websocket connections
	regularRunners := make([]*alerts.Runner, 0, numRegularUsers)
	for i := range numRegularUsers {
		runnerCfg := alerts.Config{
			User: createusers.Config{
				OrganizationID: firstUser.OrganizationID,
			},
			Roles:                 []string{},
			NotificationTimeout:   testutil.WaitLong,
			DialTimeout:           testutil.WaitLong,
			Metrics:               metrics,
			DialBarrier:           dialBarrier,
			ReceivingWatchBarrier: receivingWatchBarrier,
		}
		err := runnerCfg.Validate()
		require.NoError(t, err)

		runner := alerts.NewRunner(client, runnerCfg)
		regularRunners = append(regularRunners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, "regular-"+strconv.Itoa(i), io.Discard)
		})
	}

	// Trigger notifications by creating and deleting a user
	eg.Go(func() error {
		// Wait for all runners to connect
		dialBarrier.Wait()

		for i := 0; i < numReceivingUsers; i++ {
			smtpTrap.MustWait(runCtx).MustRelease(runCtx)
		}

		for i := 0; i < numReceivingUsers; i++ {
			err := sendInboxAlert(runCtx, t, db, inboxHandler, "receiving-user-"+strconv.Itoa(i), notificationsLib.TemplateUserAccountCreated)
			require.NoError(t, err)
			err = sendInboxAlert(runCtx, t, db, inboxHandler, "receiving-user-"+strconv.Itoa(i), notificationsLib.TemplateUserAccountDeleted)
			require.NoError(t, err)
		}

		_, w := mClock.AdvanceNext()
		w.MustWait(runCtx)

		return nil
	})

	err := eg.Wait()
	require.NoError(t, err, "runner execution with SMTP should complete successfully")

	cleanupEg, cleanupCtx := errgroup.WithContext(ctx)
	for i, runner := range receivingRunners {
		cleanupEg.Go(func() error {
			return runner.Cleanup(cleanupCtx, "receiving-"+strconv.Itoa(i), io.Discard)
		})
	}
	for i, runner := range regularRunners {
		cleanupEg.Go(func() error {
			return runner.Cleanup(cleanupCtx, "regular-"+strconv.Itoa(i), io.Discard)
		})
	}
	err = cleanupEg.Wait()
	require.NoError(t, err)

	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 1)
	require.Equal(t, firstUser.UserID, users.Users[0].ID)

	// Verify that notifications were received via both websocket and SMTP
	for _, runner := range receivingRunners {
		metrics := runner.GetMetrics()
		websocketReceiptTimes := metrics[alerts.WebsocketNotificationReceiptTimeMetric].(map[uuid.UUID]time.Time)
		smtpReceiptTimes := metrics[alerts.SMTPNotificationReceiptTimeMetric].(map[uuid.UUID]time.Time)

		require.Contains(t, websocketReceiptTimes, notificationsLib.TemplateUserAccountCreated)
		require.Contains(t, websocketReceiptTimes, notificationsLib.TemplateUserAccountDeleted)
		require.Contains(t, smtpReceiptTimes, notificationsLib.TemplateUserAccountCreated)
		require.Contains(t, smtpReceiptTimes, notificationsLib.TemplateUserAccountDeleted)
	}
}

func sendInboxAlert(ctx context.Context, t *testing.T, db database.Store, inboxHandler *dispatch.InboxHandler, username string, templateID uuid.UUID) error {
	user, err := db.GetUserByEmailOrUsername(ctx, database.GetUserByEmailOrUsernameParams{
		Username: username,
	})
	require.NoError(t, err)

	dispatchFunc, err := inboxHandler.Dispatcher(types.MessagePayload{
		UserID:          user.ID.String(),
		AlertTemplateID: templateID.String(),
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
