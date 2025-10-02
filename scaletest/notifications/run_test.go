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
		Database:                 db,
		Pubsub:                   ps,
		NotificationsEnqueuer:    enqueuer,
	})
	firstUser := coderdtest.CreateFirstUser(t, client)

	const numOwners = 2
	barrier := new(sync.WaitGroup)
	barrier.Add(numOwners + 1)
	metrics := notifications.NewMetrics(prometheus.NewRegistry())

	eg, runCtx := errgroup.WithContext(ctx)

	// Start owner runners who will receive notifications
	runners := make([]*notifications.Runner, 0, numOwners)
	for i := range numOwners {
		runnerCfg := notifications.Config{
			User: createusers.Config{
				OrganizationID: firstUser.OrganizationID,
			},
			IsOwner:             true,
			NotificationTimeout: testutil.WaitLong,
			DialTimeout:         testutil.WaitLong,
			Metrics:             metrics,
			DialBarrier:         barrier,
		}
		err := runnerCfg.Validate()
		require.NoError(t, err)

		runner := notifications.NewRunner(client, runnerCfg)
		runners = append(runners, runner)
		eg.Go(func() error {
			return runner.Run(runCtx, "owner-"+strconv.Itoa(i), io.Discard)
		})
	}

	// Trigger notifications by creating and deleting a user
	eg.Go(func() error {
		barrier.Done()
		barrier.Wait()

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
	for i, runner := range runners {
		cleanupEg.Go(func() error {
			return runner.Cleanup(cleanupCtx, strconv.Itoa(i), io.Discard)
		})
	}
	err = cleanupEg.Wait()
	require.NoError(t, err)

	// Verify that each runner received both notifications and recorded metrics
	for _, runner := range runners {
		runnerMetrics := runner.GetMetrics()
		require.Contains(t, runnerMetrics, notifications.UserCreatedNotificationLatencyMetric)
		require.Contains(t, runnerMetrics, notifications.UserDeletedNotificationLatencyMetric)
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
