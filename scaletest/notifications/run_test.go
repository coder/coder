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

	const numOwners = 2
	const numRegularUsers = 2
	ownerBarrier := new(sync.WaitGroup)
	regularBarrier := new(sync.WaitGroup)
	ownerBarrier.Add(numOwners)
	regularBarrier.Add(numRegularUsers)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	eg, runCtx := errgroup.WithContext(ctx)

	// Start owner runners who will receive notifications
	ownerRunners := make([]*notifications.Runner, 0, numOwners)
	for i := range numOwners {
		runnerCfg := notifications.Config{
			User: createusers.Config{
				OrganizationID: firstUser.OrganizationID,
			},
			IsOwner:             true,
			NotificationTimeout: testutil.WaitLong,
			DialTimeout:         testutil.WaitLong,
			Metrics:             metrics,
			DialBarrier:         ownerBarrier,
		}
		err := runnerCfg.Validate()
		require.NoError(t, err)

		runner := notifications.NewRunner(client, runnerCfg)
		ownerRunners = append(ownerRunners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, "owner-"+strconv.Itoa(i), io.Discard)
		})
	}

	// Start regular user runners who will maintain websocket connections
	regularRunners := make([]*notifications.Runner, 0, numRegularUsers)
	for i := range numRegularUsers {
		runnerCfg := notifications.Config{
			User: createusers.Config{
				OrganizationID: firstUser.OrganizationID,
			},
			IsOwner:             false,
			NotificationTimeout: testutil.WaitLong,
			DialTimeout:         testutil.WaitLong,
			Metrics:             metrics,
			DialBarrier:         regularBarrier,
			OwnerDialBarrier:    ownerBarrier,
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
		ownerBarrier.Wait()
		regularBarrier.Wait()

		newUser, err := client.CreateUserWithOrgs(runCtx, codersdk.CreateUserRequestWithOrgs{
			OrganizationIDs: []uuid.UUID{firstUser.OrganizationID},
			Email:           "test-user@coder.com",
			Username:        "test-user",
			Password:        "SomeSecurePassword!",
		})
		if err != nil {
			return xerrors.Errorf("create test user: %w", err)
		}

		if err := client.DeleteUser(runCtx, newUser.ID); err != nil {
			return xerrors.Errorf("delete test user: %w", err)
		}

		return nil
	})

	err = eg.Wait()
	require.NoError(t, err, "runner execution should complete successfully")

	cleanupEg, cleanupCtx := errgroup.WithContext(ctx)
	for i, runner := range ownerRunners {
		cleanupEg.Go(func() error {
			return runner.Cleanup(cleanupCtx, "owner-"+strconv.Itoa(i), io.Discard)
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

	// Verify that owner runners received both notifications and recorded metrics
	for _, runner := range ownerRunners {
		runnerMetrics := runner.GetMetrics()
		require.Contains(t, runnerMetrics, notifications.UserCreatedNotificationLatencyMetric)
		require.Contains(t, runnerMetrics, notifications.UserDeletedNotificationLatencyMetric)
	}

	// Verify that regular runners don't have notification metrics
	for _, runner := range regularRunners {
		runnerMetrics := runner.GetMetrics()
		require.NotContains(t, runnerMetrics, notifications.UserCreatedNotificationLatencyMetric)
		require.NotContains(t, runnerMetrics, notifications.UserDeletedNotificationLatencyMetric)
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
