package coderd_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/testutil"
)

func TestGetUserSecretByUserIDAndNameForUpdateLocks(t *testing.T) {
	t.Parallel()

	db, _ := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	user := dbgen.User(t, db, database.User{})
	secret := dbgen.UserSecret(t, db, database.UserSecret{
		UserID: user.ID, Name: "lock-me", EnvName: "LOCK_ME",
	})
	arg := database.GetUserSecretByUserIDAndNameForUpdateParams{
		UserID: user.ID, Name: secret.Name,
	}
	lock := func() (<-chan error, func()) {
		locked, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		go func() {
			done <- db.InTx(func(tx database.Store) error {
				if _, err := tx.GetUserSecretByUserIDAndNameForUpdate(ctx, arg); err != nil {
					return err
				}
				close(locked)
				<-release
				return nil
			}, nil)
		}()
		testutil.TryReceive(ctx, t, locked)
		return done, func() { close(release) }
	}

	firstDone, releaseFirst := lock()

	second := make(chan error, 1)
	go func() {
		second <- db.InTx(func(tx database.Store) error {
			_, err := tx.GetUserSecretByUserIDAndNameForUpdate(ctx, arg)
			return err
		}, nil)
	}()

	select {
	case err := <-second:
		t.Fatalf("second transaction acquired the row lock while it was held: %v", err)
	case <-time.After(testutil.IntervalMedium):
	}

	releaseFirst()
	require.NoError(t, testutil.TryReceive(ctx, t, firstDone))
	require.NoError(t, testutil.TryReceive(ctx, t, second))
}
