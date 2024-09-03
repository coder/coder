package idpsync

import (
	"context"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

type GroupParams struct {
	// SyncEnabled if false will skip syncing the user's groups
	SyncEnabled  bool
	MergedClaims jwt.MapClaims
}

func (AGPLIDPSync) GroupSyncEnabled() bool {
	// AGPL does not support syncing groups.
	return false
}

func (s AGPLIDPSync) ParseGroupClaims(_ context.Context, _ jwt.MapClaims) (GroupParams, *HTTPError) {
	return GroupParams{
		SyncEnabled: s.GroupSyncEnabled(),
	}, nil
}

// TODO: Group allowlist behavior should probably happen at this step.
func (s AGPLIDPSync) SyncGroups(ctx context.Context, db database.Store, user database.User, params GroupParams) error {
	// Nothing happens if sync is not enabled
	if !params.SyncEnabled {
		return nil
	}

	// nolint:gocritic // all syncing is done as a system user
	ctx = dbauthz.AsSystemRestricted(ctx)

	db.InTx(func(tx database.Store) error {
		userGroups, err := db.GetGroups(ctx, database.GetGroupsParams{
			HasMemberID: user.ID,
		})
		if err != nil {
			return xerrors.Errorf("get user groups: %w", err)
		}

		// Figure out which organizations the user is a member of.
		userOrgs := make(map[uuid.UUID][]database.GetGroupsRow)
		for _, g := range userGroups {
			g := g
			userOrgs[g.Group.OrganizationID] = append(userOrgs[g.Group.OrganizationID], g)
		}

		// Force each organization, we sync the groups.
		db.RemoveUserFromAllGroups(ctx, user.ID)

		return nil
	}, nil)

	//
	//tx.InTx(func(tx database.Store) error {
	//	// When setting the user's groups, it's easier to just clear their groups and re-add them.
	//	// This ensures that the user's groups are always in sync with the auth provider.
	//	 err := tx.RemoveUserFromAllGroups(ctx, user.ID)
	//	if err != nil {
	//		return err
	//	}
	//
	//	for _, org := range userOrgs {
	//
	//	}
	//
	//	return nil
	//}, nil)

	return nil
}
