package prebuilds

import (
	"context"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

func Claim(ctx context.Context, store database.Store, userID uuid.UUID) (*uuid.UUID, error) {
	var prebuildID *uuid.UUID
	err := store.InTx(func(db database.Store) error {
		// TODO: do we need this?
		//// Ensure no other replica can claim a prebuild for this user simultaneously.
		//err := store.AcquireLock(ctx, database.GenLockID(fmt.Sprintf("prebuild-user-claim-%s", userID.String())))
		//if err != nil {
		//	return xerrors.Errorf("acquire claim lock for user %q: %w", userID.String(), err)
		//}

		id, err := db.ClaimPrebuild(ctx, userID)
		if err != nil {
			return xerrors.Errorf("claim prebuild for user %q: %w", userID.String(), err)
		}

		if id != uuid.Nil {
			prebuildID = &id
		}

		return nil
	}, &database.TxOptions{
		TxIdentifier: "prebuild-claim",
	})

	return prebuildID, err
}
