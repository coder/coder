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

type EnterpriseClaimer struct {
	store database.Store
}

func NewEnterpriseClaimer(store database.Store) *EnterpriseClaimer {
	return &EnterpriseClaimer{
		store: store,
	}
}

func (c EnterpriseClaimer) Claim(
	ctx context.Context,
	userID uuid.UUID,
	name string,
	presetID uuid.UUID,
	autostartSchedule sql.NullString,
	nextStartAt sql.NullTime,
	ttl sql.NullInt64,
) (*uuid.UUID, error) {
	result, err := c.store.ClaimPrebuiltWorkspace(ctx, database.ClaimPrebuiltWorkspaceParams{
		NewUserID:         userID,
		NewName:           name,
		PresetID:          presetID,
		AutostartSchedule: autostartSchedule,
		NextStartAt:       nextStartAt,
		WorkspaceTtl:      ttl,
	})
	if err != nil {
		switch {
		// No eligible prebuilds found
		case errors.Is(err, sql.ErrNoRows):
			return nil, prebuilds.ErrNoClaimablePrebuiltWorkspaces
		default:
			return nil, xerrors.Errorf("claim prebuild for user %q: %w", userID.String(), err)
		}
	}

	return &result.ID, nil
}

func (EnterpriseClaimer) Initiator() uuid.UUID {
	return database.PrebuildsSystemUserID
}

var _ prebuilds.Claimer = &EnterpriseClaimer{}
