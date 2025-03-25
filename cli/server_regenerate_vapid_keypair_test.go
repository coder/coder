package cli_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/clitest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestRegenerateVapidKeypair(t *testing.T) {
	t.Parallel()

	t.Run("NoExistingVAPIDKeys", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)

		connectionURL, err := dbtestutil.Open(t)
		require.NoError(t, err)

		sqlDB, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		defer sqlDB.Close()

		db := database.New(sqlDB)
		// Ensure there is no existing VAPID keypair.
		rows, err := db.GetNotificationVAPIDKeys(ctx)
		require.NoError(t, err)
		require.Empty(t, rows)

		inv, _ := clitest.New(t, "server", "regenerate-vapid-keypair", "--postgres-url", connectionURL, "--yes")

		pty := ptytest.New(t)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		clitest.Start(t, inv)

		pty.ExpectMatchContext(ctx, "Regenerating VAPID keypair...")
		pty.ExpectMatchContext(ctx, "This will delete all existing push notification subscriptions.")
		pty.ExpectMatchContext(ctx, "Are you sure you want to continue? (y/N)")
		pty.WriteLine("y")
		pty.ExpectMatchContext(ctx, "VAPID keypair regenerated successfully.")

		// Ensure the VAPID keypair was created.
		keys, err := db.GetNotificationVAPIDKeys(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, keys.VapidPublicKey)
		require.NotEmpty(t, keys.VapidPrivateKey)
	})

	t.Run("ExistingVAPIDKeys", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		t.Cleanup(cancel)

		connectionURL, err := dbtestutil.Open(t)
		require.NoError(t, err)

		sqlDB, err := sql.Open("postgres", connectionURL)
		require.NoError(t, err)
		defer sqlDB.Close()

		db := database.New(sqlDB)
		for i := 0; i < 10; i++ {
			// Insert a few fake users.
			u := dbgen.User(t, db, database.User{})
			// Insert a few fake push subscriptions for each user.
			for j := 0; j < 10; j++ {
				_ = dbgen.NotificationPushSubscription(t, db, database.InsertNotificationPushSubscriptionParams{
					UserID: u.ID,
				})
			}
		}

		inv, _ := clitest.New(t, "server", "regenerate-vapid-keypair", "--postgres-url", connectionURL, "--yes")

		pty := ptytest.New(t)
		inv.Stdout = pty.Output()
		inv.Stderr = pty.Output()
		clitest.Start(t, inv)

		pty.ExpectMatchContext(ctx, "Regenerating VAPID keypair...")
		pty.ExpectMatchContext(ctx, "This will delete all existing push notification subscriptions.")
		pty.ExpectMatchContext(ctx, "Are you sure you want to continue? (y/N)")
		pty.WriteLine("y")
		pty.ExpectMatchContext(ctx, "VAPID keypair regenerated successfully.")

		// Ensure the VAPID keypair was created.
		keys, err := db.GetNotificationVAPIDKeys(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, keys.VapidPublicKey)
		require.NotEmpty(t, keys.VapidPrivateKey)

		// Ensure the push subscriptions were deleted.
		var count int64
		rows, err := sqlDB.QueryContext(ctx, "SELECT COUNT(*) FROM notification_push_subscriptions")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = rows.Close()
		})
		require.True(t, rows.Next())
		require.NoError(t, rows.Scan(&count))
		require.Equal(t, int64(0), count)
	})
}
