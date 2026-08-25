package coderd_test

import (
	"testing"

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

	// The first transaction holds the row lock until released.
	locked, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- db.InTx(func(tx database.Store) error {
			if _, err := tx.GetUserSecretByUserIDAndNameForUpdate(ctx, arg); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		}, nil)
	}()
	testutil.TryReceive(ctx, t, locked)

	second := make(chan error, 1)
	go func() {
		second <- db.InTx(func(tx database.Store) error {
			_, err := tx.GetUserSecretByUserIDAndNameForUpdate(ctx, arg)
			return err
		}, nil)
	}()

	// Wait until the second transaction is observably blocked on the row lock,
	// rather than inferring it from a fixed delay. A waiter for SELECT ... FOR
	// UPDATE takes a granted "tuple" lock on the relation and then waits on the
	// holder's transaction ID, so it appears in pg_locks as a backend holding
	// both rows. Asserting the wait directly stops this from passing vacuously
	// when the second goroutine is merely slow to reach the query.
	require.Eventually(t, func() bool {
		locks, err := db.PGLocks(ctx)
		if err != nil {
			return false
		}
		waitingOnRow := make(map[int]bool)
		for _, l := range locks {
			if l.LockType != nil && *l.LockType == "tuple" &&
				l.RelationName != nil && *l.RelationName == "user_secrets" {
				waitingOnRow[l.PID] = true
			}
		}
		for _, l := range locks {
			if !l.Granted && waitingOnRow[l.PID] &&
				l.LockType != nil && *l.LockType == "transactionid" {
				return true
			}
		}
		return false
	}, testutil.WaitShort, testutil.IntervalFast, "second transaction must block on the user_secrets row lock")

	// With the second transaction proven blocked, it must not have completed
	// while the row lock is still held.
	select {
	case err := <-second:
		t.Fatalf("second transaction acquired the row lock while it was held: %v", err)
	default:
	}

	close(release)
	require.NoError(t, testutil.TryReceive(ctx, t, first))
	require.NoError(t, testutil.TryReceive(ctx, t, second))
}
