package coderd

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

var errFirstUserExists = xerrors.New("the initial user has already been created")

// lockFirstUserSetup takes the first user setup lock and returns the
// user count while the lock is held. The lock is released when tx ends.
//
// tx must use the default isolation level. At repeatable read or
// serializable, the count would come from a snapshot taken before the
// wait and could miss a user that another request committed.
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
