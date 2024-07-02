package notifications_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func setup(t *testing.T) (context.Context, slog.Logger, database.Store, *pubsub.PGPubsub) {
	t.Helper()

	connectionURL, closeFunc, err := dbtestutil.Open()
	require.NoError(t, err)
	t.Cleanup(closeFunc)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitSuperLong)
	t.Cleanup(cancel)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true, IgnoredErrorIs: []error{}}).Leveled(slog.LevelDebug)

	sqlDB, err := sql.Open("postgres", connectionURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	db := database.New(sqlDB)
	ps, err := pubsub.New(ctx, logger, sqlDB, connectionURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, ps.Close())
	})

	// nolint:gocritic // unit tests.
	return dbauthz.AsSystemRestricted(ctx), logger, db, ps
}

func defaultNotificationsConfig(method database.NotificationMethod) codersdk.NotificationsConfig {
	return codersdk.NotificationsConfig{
		Method:              serpent.String(method),
		MaxSendAttempts:     5,
		RetryInterval:       serpent.Duration(time.Minute * 5),
		StoreSyncInterval:   serpent.Duration(time.Second * 2),
		StoreSyncBufferSize: 50,
		LeasePeriod:         serpent.Duration(time.Minute * 2),
		LeaseCount:          10,
		FetchInterval:       serpent.Duration(time.Second * 10),
		DispatchTimeout:     serpent.Duration(time.Minute),
		SMTP:                codersdk.NotificationsEmailConfig{},
		Webhook:             codersdk.NotificationsWebhookConfig{},
	}
}

func defaultHelpers() map[string]any {
	return map[string]any{
		"base_url": func() string { return "http://test.com" },
	}
}
