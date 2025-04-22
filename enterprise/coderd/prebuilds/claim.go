package prebuilds

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/prebuilds"
)

type EnterpriseClaimer struct{}

func (_ EnterpriseClaimer) Claim(
	ctx context.Context,
	store database.Store,
	userID uuid.UUID,
	name string,
	presetID uuid.UUID,
) (*uuid.UUID, error) {
	result, err := store.ClaimPrebuiltWorkspace(ctx, database.ClaimPrebuiltWorkspaceParams{
		NewUserID: userID,
		NewName:   name,
		PresetID:  presetID,
	})
	if err != nil {
		switch {
		// No eligible prebuilds found
		case errors.Is(err, sql.ErrNoRows):
			// Exit, this will result in a nil prebuildID being returned, which is fine
			return nil, nil
		default:
			return nil, xerrors.Errorf("claim prebuild for user %q: %w", userID.String(), err)
		}
	}

	return &result.ID, nil
}

func (_ EnterpriseClaimer) Initiator() uuid.UUID {
	return prebuilds.SystemUserID
}

var _ prebuilds.Claimer = &EnterpriseClaimer{}
