package coderd

import (
	"context"
	"database/sql"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

var errFirstUserExists = xerrors.New("the initial user has already been created")

// firstUserSetupTxOptions names the isolation level that
// lockFirstUserSetup needs. At read committed the count sees a user
// that another request committed while this one waited for the lock.
// At a higher level the count comes from a snapshot taken before the
// wait, and a Postgres server can make a higher level its default.
// Built per call because InTx writes to the options it receives.
func firstUserSetupTxOptions() *database.TxOptions {
	return &database.TxOptions{Isolation: sql.LevelReadCommitted}
}

// lockFirstUserSetup takes the first user setup lock and returns the
// user count while the lock is held. The lock is released when tx ends.
// tx must be started with firstUserSetupTxOptions.
func lockFirstUserSetup(ctx context.Context, tx database.Store) (int64, error) {
	if err := tx.AcquireLock(ctx, database.LockIDFirstUserSetup); err != nil {
		return 0, xerrors.Errorf("acquire first user setup lock: %w", err)
	}
	//nolint:gocritic // Getting user count is a system function.
	userCount, err := tx.GetUserCount(dbauthz.AsSystemRestricted(ctx), false)
	if err != nil {
		return 0, xerrors.Errorf("get user count: %w", err)
	}
	return userCount, nil
}
