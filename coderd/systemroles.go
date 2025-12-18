package coderd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/rolestore"
)

// ReconcileOrgMemberRoles ensures all orgs have an org-member system
// role stored in the DB with permissions reflecting current RBAC
// resources and the org's workspace_sharing_disabled setting.
//
//  1. Create org-member system roles for orgs that lack one
//  2. Update permissions when new RBAC resources are added to the codebase
//  3. Ensure permissions match each org's workspace sharing setting
//
// Uses PostgreSQL advisory lock (LockIDReconcileOrgMemberRoles) to
// safely handle multi-instance deployments. Other coderd instances
// will block until the first instance completes, then find
// permissions already up-to-date.
//
// Uses set-based comparison to avoid unnecessary database writes when
// permissions haven't changed.
func ReconcileOrgMemberRoles(ctx context.Context, logger slog.Logger, db database.Store) error {
	//nolint:gocritic // We need to manage system roles
	sysCtx := dbauthz.AsSystemRestricted(ctx)

	return db.InTx(func(tx database.Store) error {
		// Acquire advisory lock to prevent concurrent updates from
		// multiple coderd instances. Other instances will block here
		// until we release the lock (when this transaction commits).
		err := tx.AcquireLock(sysCtx, database.LockIDReconcileOrgMemberRoles)
		if err != nil {
			return xerrors.Errorf("acquire reconcile org-member roles lock: %w", err)
		}

		// Fetch all organizations with their workspace sharing setting.
		orgs, err := tx.GetOrganizations(sysCtx, database.GetOrganizationsParams{})
		if err != nil {
			return xerrors.Errorf("fetch organizations: %w", err)
		}

		// Fetch all system roles.
		// TODO:(geokat) Add a way to filter by name in SQL instead.
		systemRoles, err := tx.CustomRoles(sysCtx, database.CustomRolesParams{
			IncludeSystemRoles: true,
		})
		if err != nil {
			return xerrors.Errorf("fetch custom roles: %w", err)
		}

		// Find org-member roles and index by organization ID for quick lookup.
		rolesByOrg := make(map[uuid.UUID]database.CustomRole)
		for _, role := range systemRoles {
			if role.IsSystem && role.Name == rbac.RoleOrgMember() && role.OrganizationID.Valid {
				rolesByOrg[role.OrganizationID.UUID] = role
			}
		}

		for _, org := range orgs {
			role, exists := rolesByOrg[org.ID]
			if !exists {
				// Role doesn't exist; create it. This could happen due to a
				// rolling upgrade race condition where an old instance creates
				// an org after the migration has already run.
				logger.Warn(sysCtx, "org-member role missing for organization, creating",
					slog.F("organization_id", org.ID),
					slog.F("organization_name", org.Name))
				if err := createOrgMemberRoleForOrg(sysCtx, tx, org); err != nil {
					return xerrors.Errorf("create org-member role for org %s: %w", org.ID, err)
				}
				continue
			}

			// Generate expected perms based on org's workspace sharing setting.
			expectedOrgPerms, expectedMemberPerms := rbac.OrgMemberPermissions(org.WorkspaceSharingDisabled)

			storedOrgPerms := rolestore.ConvertDBPermissions(role.OrgPermissions)
			storedMemberPerms := rolestore.ConvertDBPermissions(role.MemberPermissions)

			// Compare using set-based comparison (order doesn't matter).
			orgPermsMatch := rbac.PermissionsEqual(expectedOrgPerms, storedOrgPerms)
			memberPermsMatch := rbac.PermissionsEqual(expectedMemberPerms, storedMemberPerms)

			if !orgPermsMatch || !memberPermsMatch {
				logger.Info(sysCtx, "updating org-member role permissions",
					slog.F("organization_id", org.ID),
					slog.F("organization_name", org.Name),
					slog.F("org_perms_changed", !orgPermsMatch),
					slog.F("member_perms_changed", !memberPermsMatch))

				_, err = tx.UpdateCustomRole(sysCtx, database.UpdateCustomRoleParams{
					Name:              role.Name,
					OrganizationID:    role.OrganizationID,
					DisplayName:       role.DisplayName,
					SitePermissions:   role.SitePermissions,
					OrgPermissions:    rolestore.ConvertPermissionsToDB(expectedOrgPerms),
					UserPermissions:   role.UserPermissions,
					MemberPermissions: rolestore.ConvertPermissionsToDB(expectedMemberPerms),
				})
				if err != nil {
					return xerrors.Errorf("update org-member role for org %s: %w", org.ID, err)
				}
			}
		}

		return nil
	}, nil)
}

// createOrgMemberRoleForOrg creates the org-member system role for an
// org. This is a fallback for organizations that don't have that role
// which can happen due to, say, race conditions during a rolling
// upgrade in multi-instance scenario.
func createOrgMemberRoleForOrg(ctx context.Context, tx database.Store, org database.Organization) error {
	orgPerms, memberPerms := rbac.OrgMemberPermissions(org.WorkspaceSharingDisabled)

	_, err := tx.InsertCustomRole(ctx, database.InsertCustomRoleParams{
		Name:              rbac.RoleOrgMember(),
		DisplayName:       "",
		OrganizationID:    uuid.NullUUID{UUID: org.ID, Valid: true},
		SitePermissions:   database.CustomRolePermissions{},
		OrgPermissions:    rolestore.ConvertPermissionsToDB(orgPerms),
		UserPermissions:   database.CustomRolePermissions{},
		MemberPermissions: rolestore.ConvertPermissionsToDB(memberPerms),
		IsSystem:          true,
	})
	if err != nil {
		return xerrors.Errorf("insert org-member role: %w", err)
	}

	return nil
}
