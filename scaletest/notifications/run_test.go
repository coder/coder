package notifications_test

import (
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	notificationsLib "github.com/coder/coder/v2/coderd/notifications"
	"github.com/coder/coder/v2/coderd/notifications/dispatch"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/notifications"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestRun(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := testutil.Logger(t)
	db, ps := dbtestutil.NewDB(t)

	// Setup notifications manager with inbox handler
	cfg := defaultNotificationsConfig(database.NotificationMethodSmtp)
	mgr, err := notificationsLib.NewManager(
		cfg,
		db,
		ps,
		defaultHelpers(),
		notificationsLib.NewMetrics(prometheus.NewRegistry()),
		logger.Named("manager"),
	)
	require.NoError(t, err)

	mgr.WithHandlers(map[database.NotificationMethod]notificationsLib.Handler{
		database.NotificationMethodInbox: dispatch.NewInboxHandler(logger.Named("inbox"), db, ps),
	})
	t.Cleanup(func() {
		assert.NoError(t, mgr.Stop(dbauthz.AsNotifier(ctx)))
	})
	mgr.Run(dbauthz.AsNotifier(ctx))

	enqueuer, err := notificationsLib.NewStoreEnqueuer(
		cfg,
		db,
		defaultHelpers(),
		logger.Named("enqueuer"),
		quartz.NewReal(),
	)
	require.NoError(t, err)

	client := coderdtest.New(t, &coderdtest.Options{
		Database:              db,
		Pubsub:                ps,
		NotificationsEnqueuer: enqueuer,
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

	expectedNotifications := map[uuid.UUID]chan time.Time{
		notificationsLib.TemplateUserAccountCreated: make(chan time.Time, 1),
		notificationsLib.TemplateUserAccountDeleted: make(chan time.Time, 1),
	}

	// Start receiving runners who will receive notifications
	receivingRunners := make([]*notifications.Runner, 0, numReceivingUsers)
	for i := range numReceivingUsers {
		runnerCfg := notifications.Config{
			User: createusers.Config{
				OrganizationID: firstUser.OrganizationID,
			},
			Roles:                 []string{codersdk.RoleOwner},
			NotificationTimeout:   testutil.WaitLong,
			DialTimeout:           testutil.WaitLong,
			Metrics:               metrics,
			DialBarrier:           dialBarrier,
			ReceivingWatchBarrier: receivingWatchBarrier,
			ExpectedNotifications: expectedNotifications,
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

		createTime := time.Now()
		newUser, err := client.CreateUserWithOrgs(runCtx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{firstUser.OrganizationID},
			Email:           "test-user@coder.com",
			Username:        "test-user",
			Password:        "SomeSecurePassword!",
		})
		if err != nil {
			return xerrors.Errorf("create test user: %w", err)
		}
		expectedNotifications[notificationsLib.TemplateUserAccountCreated] <- createTime

		deleteTime := time.Now()
		if err := client.DeleteUser(runCtx, newUser.ID); err != nil {
			return xerrors.Errorf("delete test user: %w", err)
		}
		expectedNotifications[notificationsLib.TemplateUserAccountDeleted] <- deleteTime

		close(expectedNotifications[notificationsLib.TemplateUserAccountCreated])
		close(expectedNotifications[notificationsLib.TemplateUserAccountDeleted])

		return nil
	})

	err = eg.Wait()
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
		runnerMetrics := runner.GetMetrics()[notifications.NotificationDeliveryLatencyMetric].(map[uuid.UUID]time.Duration)
		require.Contains(t, runnerMetrics, notificationsLib.TemplateUserAccountCreated)
		require.Contains(t, runnerMetrics, notificationsLib.TemplateUserAccountDeleted)
	}
}

func defaultNotificationsConfig(method database.NotificationMethod) codersdk.NotificationsConfig {
	return codersdk.NotificationsConfig{
		Method:              serpent.String(method),
		MaxSendAttempts:     5,
		FetchInterval:       serpent.Duration(time.Millisecond * 100),
		StoreSyncInterval:   serpent.Duration(time.Millisecond * 200),
		LeasePeriod:         serpent.Duration(time.Second * 10),
		DispatchTimeout:     serpent.Duration(time.Second * 5),
		RetryInterval:       serpent.Duration(time.Millisecond * 50),
		LeaseCount:          10,
		StoreSyncBufferSize: 50,
		Inbox: codersdk.NotificationsInboxConfig{
			Enabled: serpent.Bool(true),
		},
	}
}

func defaultHelpers() map[string]any {
	return map[string]any{
		"base_url":     func() string { return "http://test.com" },
		"current_year": func() string { return "2024" },
		"logo_url":     func() string { return "https://coder.com/coder-logo-horizontal.png" },
		"app_name":     func() string { return "Coder" },
	}
}
