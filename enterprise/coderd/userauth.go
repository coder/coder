package coderd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
)

// nolint: revive
func (api *API) setUserGroups(ctx context.Context, logger slog.Logger, db database.Store, userID uuid.UUID, groupNames []string, createMissingGroups bool) error {
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

		if createMissingGroups {
			// This is the system creating these additional groups, so we use the system restricted context.
			// nolint:gocritic
			created, err := tx.InsertMissingGroups(dbauthz.AsSystemRestricted(ctx), database.InsertMissingGroupsParams{
				OrganizationID: orgs[0].ID,
				GroupNames:     groupNames,
				Source:         database.GroupSourceOidc,
			})
			if err != nil {
				return xerrors.Errorf("insert missing groups: %w", err)
			}
			if len(created) > 0 {
				logger.Debug(ctx, "auto created missing groups",
					slog.F("org_id", orgs[0].ID),
					slog.F("created", created),
				)
			}
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

func (api *API) setUserSiteRoles(ctx context.Context, logger slog.Logger, db database.Store, userID uuid.UUID, roles []string) error {
	api.entitlementsMu.RLock()
	enabled := api.entitlements.Features[codersdk.FeatureUserRoleManagement].Enabled
	api.entitlementsMu.RUnlock()

	if !enabled {
		logger.Warn(ctx, "attempted to assign OIDC user roles without enterprise entitlement, roles left unchanged",
			slog.F("user_id", userID), slog.F("roles", roles),
		)
		return nil
	}

	// Should this be feature protected?
	return db.InTx(func(tx database.Store) error {
		_, err := coderd.UpdateSiteUserRoles(ctx, db, database.UpdateUserRolesParams{
			GrantedRoles: roles,
			ID:           userID,
		})
		if err != nil {
			return xerrors.Errorf("set user roles(%s): %w", userID.String(), err)
		}

		return nil
	}, nil)
}
