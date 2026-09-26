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
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
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
	dialBarrier := new(sync.WaitGroup)
	receivingWatchBarrier := new(sync.WaitGroup)
	dialBarrier.Add(numReceivingUsers + numRegularUsers)
	receivingWatchBarrier.Add(numReceivingUsers)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	eg, runCtx := errgroup.WithContext(ctx)

	const numDeletions = 2

	// Start receiving runners who will receive notifications
	receivingRunners := make([]*notifications.Runner, 0, numReceivingUsers)
	receivingUsernames := make([]string, 0, numReceivingUsers)
	for i := range numReceivingUsers {
		userClient, user := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID, rbac.RoleOwner())
		receivingUsernames = append(receivingUsernames, user.Username)
		runnerCfg := notifications.Config{
			SessionToken:          userClient.SessionToken(),
			PreCreatedUser:        user,
			NotificationTimeout:   testutil.WaitLong,
			DialTimeout:           testutil.WaitLong,
			Metrics:               metrics,
			DialBarrier:           dialBarrier,
			ReceivingWatchBarrier: receivingWatchBarrier,
			IsTemplateAdmin:       true,
			ExpectedDeletions:     numDeletions,
		}
		err := runnerCfg.Validate()
		require.NoError(t, err)

		runner := notifications.NewRunner(client, runnerCfg)
		receivingRunners = append(receivingRunners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, "receiving-"+strconv.Itoa(i), io.Discard)
		})
	}

	// Start regular user runners who will maintain websocket connections
	regularRunners := make([]*notifications.Runner, 0, numRegularUsers)
	for i := range numRegularUsers {
		userClient, user := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		runnerCfg := notifications.Config{
			SessionToken:          userClient.SessionToken(),
			PreCreatedUser:        user,
			NotificationTimeout:   testutil.WaitLong,
			DialTimeout:           testutil.WaitLong,
			Metrics:               metrics,
			DialBarrier:           dialBarrier,
			ReceivingWatchBarrier: receivingWatchBarrier,
		}
		err := runnerCfg.Validate()
		require.NoError(t, err)

		runner := notifications.NewRunner(client, runnerCfg)
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
			for range numDeletions {
				err := sendInboxNotification(runCtx, t, db, inboxHandler, receivingUsernames[i], notificationsLib.TemplateTemplateDeleted)
				require.NoError(t, err)
			}
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

	// The runner reuses existing users and never deletes them, so every
	// pre-created user remains alongside the first user.
	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 1+numReceivingUsers+numRegularUsers)

	for _, runner := range receivingRunners {
		metrics := runner.GetMetrics()
		websocketDeletionReceiptTimes := metrics[notifications.WebsocketNotificationReceiptTimeMetric].([]time.Time)

		require.Len(t, websocketDeletionReceiptTimes, numDeletions)
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

	const numDeletions = 2

	smtpAPIMux := http.NewServeMux()
	smtpAPIMux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		summaries := make([]smtpmock.EmailSummary, 0, numDeletions)
		for i := range numDeletions {
			summaries = append(summaries, smtpmock.EmailSummary{
				Subject:                "TemplateTemplateDeleted",
				Date:                   time.Now(),
				MessageID:              strconv.Itoa(i),
				NotificationTemplateID: notificationsLib.TemplateTemplateDeleted,
			})
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
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	eg, runCtx := errgroup.WithContext(ctx)

	mClock := quartz.NewMock(t)
	smtpTrap := mClock.Trap().TickerFunc("smtp")
	defer smtpTrap.Close()

	httpClient := &http.Client{}

	// Start receiving runners who will receive notifications
	receivingRunners := make([]*notifications.Runner, 0, numReceivingUsers)
	receivingUsernames := make([]string, 0, numReceivingUsers)
	for i := range numReceivingUsers {
		userClient, user := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID, rbac.RoleOwner())
		receivingUsernames = append(receivingUsernames, user.Username)
		runnerCfg := notifications.Config{
			SessionToken:          userClient.SessionToken(),
			PreCreatedUser:        user,
			NotificationTimeout:   testutil.WaitLong,
			DialTimeout:           testutil.WaitLong,
			Metrics:               metrics,
			DialBarrier:           dialBarrier,
			ReceivingWatchBarrier: receivingWatchBarrier,
			IsTemplateAdmin:       true,
			ExpectedDeletions:     numDeletions,
			SMTPApiURL:            smtpAPIServer.URL,
			SMTPRequestTimeout:    testutil.WaitLong,
			SMTPHttpClient:        httpClient,
		}
		err := runnerCfg.Validate()
		require.NoError(t, err)

		runner := notifications.NewRunner(client, runnerCfg).WithClock(mClock)
		receivingRunners = append(receivingRunners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, "receiving-"+strconv.Itoa(i), io.Discard)
		})
	}

	// Start regular user runners who will maintain websocket connections
	regularRunners := make([]*notifications.Runner, 0, numRegularUsers)
	for i := range numRegularUsers {
		userClient, user := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
		runnerCfg := notifications.Config{
			SessionToken:          userClient.SessionToken(),
			PreCreatedUser:        user,
			NotificationTimeout:   testutil.WaitLong,
			DialTimeout:           testutil.WaitLong,
			Metrics:               metrics,
			DialBarrier:           dialBarrier,
			ReceivingWatchBarrier: receivingWatchBarrier,
		}
		err := runnerCfg.Validate()
		require.NoError(t, err)

		runner := notifications.NewRunner(client, runnerCfg)
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
			for range numDeletions {
				err := sendInboxNotification(runCtx, t, db, inboxHandler, receivingUsernames[i], notificationsLib.TemplateTemplateDeleted)
				require.NoError(t, err)
			}
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

	// The runner reuses existing users and never deletes them, so every
	// pre-created user remains alongside the first user.
	users, err := client.Users(ctx, codersdk.UsersRequest{})
	require.NoError(t, err)
	require.Len(t, users.Users, 1+numReceivingUsers+numRegularUsers)

	// Verify that notifications were received via both websocket and SMTP
	for _, runner := range receivingRunners {
		metrics := runner.GetMetrics()
		websocketDeletionReceiptTimes := metrics[notifications.WebsocketNotificationReceiptTimeMetric].([]time.Time)
		smtpReceiptTimes := metrics[notifications.SMTPNotificationReceiptTimeMetric].([]time.Time)

		require.Len(t, websocketDeletionReceiptTimes, numDeletions)
		require.Len(t, smtpReceiptTimes, numDeletions)
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
