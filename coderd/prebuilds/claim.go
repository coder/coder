package prebuilds

import (
	"context"
	"database/sql"
	"errors"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

func Claim(ctx context.Context, store database.Store, userID uuid.UUID, name string) (*uuid.UUID, error) {
	var prebuildID *uuid.UUID
	err := store.InTx(func(db database.Store) error {
		// TODO: do we need this?
		//// Ensure no other replica can claim a prebuild for this user simultaneously.
		//err := store.AcquireLock(ctx, database.GenLockID(fmt.Sprintf("prebuild-user-claim-%s", userID.String())))
		//if err != nil {
		//	return xerrors.Errorf("acquire claim lock for user %q: %w", userID.String(), err)
		//}

		result, err := db.ClaimPrebuild(ctx, database.ClaimPrebuildParams{
			NewUserID: userID,
			NewName:   name,
		})
		if err != nil {
			switch {
			// No eligible prebuilds found
			case errors.Is(err, sql.ErrNoRows):
				// Exit, this will result in a nil prebuildID being returned, which is fine
				return nil
			default:
				return xerrors.Errorf("claim prebuild for user %q: %w", userID.String(), err)
			}
		}

		prebuildID = &result.ID

		return nil
	}, &database.TxOptions{
		TxIdentifier: "prebuild-claim",
	})

	return prebuildID, err
}
