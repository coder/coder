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
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
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

	expectedNotificationsIDs := map[uuid.UUID]struct{}{
		notificationsLib.TemplateUserAccountCreated: {},
		notificationsLib.TemplateUserAccountDeleted: {},
	}

	// Start receiving runners who will receive notifications
	receivingRunners := make([]*notifications.Runner, 0, numReceivingUsers)
	for i := range numReceivingUsers {
		runnerCfg := notifications.Config{
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

		runner := notifications.NewRunner(client, runnerCfg)
		receivingRunners = append(receivingRunners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, "receiving-"+strconv.Itoa(i), io.Discard)
		})
	}

	// Start regular user runners who will maintain websocket connections
	regularRunners := make([]*notifications.Runner, 0, numRegularUsers)
	for i := range numRegularUsers {
		runnerCfg := notifications.Config{
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
			err := sendInboxNotification(runCtx, t, db, inboxHandler, "receiving-user-"+strconv.Itoa(i), notificationsLib.TemplateUserAccountCreated)
			require.NoError(t, err)
			err = sendInboxNotification(runCtx, t, db, inboxHandler, "receiving-user-"+strconv.Itoa(i), notificationsLib.TemplateUserAccountDeleted)
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
		websocketReceiptTimes := metrics[notifications.WebsocketNotificationReceiptTimeMetric].(map[uuid.UUID]time.Time)

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
				Subject:                "TemplateUserAccountCreated",
				Date:                   time.Now(),
				NotificationTemplateID: notificationsLib.TemplateUserAccountCreated,
			},
			{
				Subject:                "TemplateUserAccountDeleted",
				Date:                   time.Now(),
				NotificationTemplateID: notificationsLib.TemplateUserAccountDeleted,
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
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

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
	receivingRunners := make([]*notifications.Runner, 0, numReceivingUsers)
	for i := range numReceivingUsers {
		runnerCfg := notifications.Config{
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

		runner := notifications.NewRunner(client, runnerCfg).WithClock(mClock)
		receivingRunners = append(receivingRunners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, "receiving-"+strconv.Itoa(i), io.Discard)
		})
	}

	// Start regular user runners who will maintain websocket connections
	regularRunners := make([]*notifications.Runner, 0, numRegularUsers)
	for i := range numRegularUsers {
		runnerCfg := notifications.Config{
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
			err := sendInboxNotification(runCtx, t, db, inboxHandler, "receiving-user-"+strconv.Itoa(i), notificationsLib.TemplateUserAccountCreated)
			require.NoError(t, err)
			err = sendInboxNotification(runCtx, t, db, inboxHandler, "receiving-user-"+strconv.Itoa(i), notificationsLib.TemplateUserAccountDeleted)
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
		websocketReceiptTimes := metrics[notifications.WebsocketNotificationReceiptTimeMetric].(map[uuid.UUID]time.Time)
		smtpReceiptTimes := metrics[notifications.SMTPNotificationReceiptTimeMetric].(map[uuid.UUID]time.Time)

		require.Contains(t, websocketReceiptTimes, notificationsLib.TemplateUserAccountCreated)
		require.Contains(t, websocketReceiptTimes, notificationsLib.TemplateUserAccountDeleted)
		require.Contains(t, smtpReceiptTimes, notificationsLib.TemplateUserAccountCreated)
		require.Contains(t, smtpReceiptTimes, notificationsLib.TemplateUserAccountDeleted)
	}
}

// TestRunReuse exercises the reuse path of Run: given a pre-created user and
// session token, the runner connects as that user without creating one, receives
// its notification, and leaves the user in place on cleanup.
func TestRunReuse(t *testing.T) {
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

	// Pre-create the user the runner will reuse, and mint a token to connect as,
	// mirroring what the CLI does before starting reuse runners.
	const reuseUsername = "scaletest-reuse-0"
	_, reused := coderdtest.CreateAnotherUserMutators(t, client, firstUser.OrganizationID, nil,
		func(r *codersdk.CreateUserRequestWithOrgs) {
			r.Username = reuseUsername
			r.Email = reuseUsername + "@scaletest.local"
		})
	tokenRes, err := client.CreateToken(ctx, reused.ID.String(), codersdk.CreateTokenRequest{
		TokenName: "scaletest-notifications",
		Lifetime:  testutil.WaitLong,
	})
	require.NoError(t, err)

	dialBarrier := new(sync.WaitGroup)
	receivingWatchBarrier := new(sync.WaitGroup)
	dialBarrier.Add(1)
	receivingWatchBarrier.Add(1)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	expectedNotificationsIDs := map[uuid.UUID]struct{}{
		notificationsLib.TemplateUserAccountCreated: {},
	}

	runnerCfg := notifications.Config{
		PreCreatedUser:           &reused,
		SessionToken:             tokenRes.Key,
		NotificationTimeout:      testutil.WaitLong,
		DialTimeout:              testutil.WaitLong,
		Metrics:                  metrics,
		DialBarrier:              dialBarrier,
		ReceivingWatchBarrier:    receivingWatchBarrier,
		ExpectedNotificationsIDs: expectedNotificationsIDs,
	}
	require.NoError(t, runnerCfg.Validate())
	runner := notifications.NewRunner(client, runnerCfg)

	eg, runCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return runner.Run(runCtx, "reuse-0", io.Discard)
	})
	eg.Go(func() error {
		dialBarrier.Wait()
		return sendInboxNotification(runCtx, t, db, inboxHandler, reuseUsername, notificationsLib.TemplateUserAccountCreated)
	})
	require.NoError(t, eg.Wait(), "reuse runner should complete successfully")

	// Cleanup must not delete a reused user: the runner never created one.
	require.NoError(t, runner.Cleanup(ctx, "reuse-0", io.Discard))
	got, err := client.User(ctx, reused.ID.String())
	require.NoError(t, err)
	require.Equal(t, reused.ID, got.ID)

	receiptTimes := runner.GetMetrics()[notifications.WebsocketNotificationReceiptTimeMetric].(map[uuid.UUID]time.Time)
	require.Contains(t, receiptTimes, notificationsLib.TemplateUserAccountCreated)
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
