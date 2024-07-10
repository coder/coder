package notifications_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func setup(t *testing.T) (context.Context, slog.Logger, database.Store) {
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

	// nolint:gocritic // unit tests.
	return dbauthz.AsSystemRestricted(ctx), logger, database.New(sqlDB)
}

func setupInMemory(t *testing.T) (context.Context, slog.Logger, database.Store) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	t.Cleanup(cancel)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true, IgnoredErrorIs: []error{}}).Leveled(slog.LevelDebug)

	// nolint:gocritic // unit tests.
	return dbauthz.AsSystemRestricted(ctx), logger, dbmem.New()
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

func createSampleUser(t *testing.T, db database.Store) database.User {
	return dbgen.User(t, db, database.User{
		Email:    "bob@coder.com",
		Username: "bob",
	})
}
