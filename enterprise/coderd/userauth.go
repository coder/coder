package coderd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

func (api *API) setUserGroups(ctx context.Context, db database.Store, userID uuid.UUID, groupNames []string) error {
	api.entitlementsMu.RLock()
	enabled := api.entitlements.Features[codersdk.FeatureTemplateRBAC].Enabled
	api.entitlementsMu.RUnlock()

	if !enabled {
		return nil
	}

	return db.InTx(func(tx database.Store) error {
		orgs, err := tx.GetOrganizationsByUserID(ctx, userID)
		if err != nil {
			return xerrors.Errorf("get user orgs: %w", err)
		}
		if len(orgs) != 1 {
			return xerrors.Errorf("expected 1 org, got %d", len(orgs))
		}

		// Delete all groups the user belongs to.
		err = tx.DeleteGroupMembersByOrgAndUser(ctx, database.DeleteGroupMembersByOrgAndUserParams{
			UserID:         userID,
			OrganizationID: orgs[0].ID,
		})
		if err != nil {
			return xerrors.Errorf("delete user groups: %w", err)
		}

		// Re-add the user to all groups returned by the auth provider.
		err = tx.InsertUserGroupsByName(ctx, database.InsertUserGroupsByNameParams{
			UserID:         userID,
			OrganizationID: orgs[0].ID,
			GroupNames:     groupNames,
		})
		if err != nil {
			return xerrors.Errorf("insert user groups: %w", err)
		}

		return nil
	}, nil)
}
