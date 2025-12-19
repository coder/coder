package rolestore

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/syncmap"
)

type customRoleCtxKey struct{}

// CustomRoleMW adds a custom role cache on the ctx to prevent duplicate
// db fetches.
func CustomRoleMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(CustomRoleCacheContext(r.Context()))
		next.ServeHTTP(w, r)
	})
}

// CustomRoleCacheContext prevents needing to lookup custom roles within the
// same request lifecycle. Optimizing this to span requests should be done
// in the future.
func CustomRoleCacheContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, customRoleCtxKey{}, syncmap.New[string, rbac.Role]())
}

func roleCache(ctx context.Context) *syncmap.Map[string, rbac.Role] {
	c, ok := ctx.Value(customRoleCtxKey{}).(*syncmap.Map[string, rbac.Role])
	if !ok {
		return syncmap.New[string, rbac.Role]()
	}
	return c
}

// Expand will expand built in roles, and fetch custom roles from the database.
// If a custom role is defined, but does not exist, the role will be omitted on
// the response. This means deleted roles are silently dropped.
func Expand(ctx context.Context, db database.Store, names []rbac.RoleIdentifier) (rbac.Roles, error) {
	if len(names) == 0 {
		// That was easy
		return []rbac.Role{}, nil
	}

	cache := roleCache(ctx)
	lookup := make([]rbac.RoleIdentifier, 0)
	roles := make([]rbac.Role, 0, len(names))

	for _, name := range names {
		// Remove any built in roles
		expanded, err := rbac.RoleByName(name)
		if err == nil {
			roles = append(roles, expanded)
			continue
		}

		// Check custom role cache
		customRole, ok := cache.Load(name.String())
		if ok {
			roles = append(roles, customRole)
			continue
		}

		// Defer custom role lookup
		lookup = append(lookup, name)
	}

	if len(lookup) > 0 {
		lookupArgs := make([]database.NameOrganizationPair, 0, len(lookup))
		for _, name := range lookup {
			lookupArgs = append(lookupArgs, database.NameOrganizationPair{
				Name:           name.Name,
				OrganizationID: name.OrganizationID,
			})
		}

		// If some roles are missing from the database, they are omitted from
		// the expansion. These roles are no-ops. Should we raise some kind of
		// warning when this happens?
		dbroles, err := db.CustomRoles(ctx, database.CustomRolesParams{
			LookupRoles:        lookupArgs,
			ExcludeOrgRoles:    false,
			OrganizationID:     uuid.Nil,
			IncludeSystemRoles: true,
		})
		if err != nil {
			return nil, xerrors.Errorf("fetch custom roles: %w", err)
		}

		// convert dbroles -> roles
		for _, dbrole := range dbroles {
			converted, err := ConvertDBRole(dbrole)
			if err != nil {
				return nil, xerrors.Errorf("convert db role %q: %w", dbrole.Name, err)
			}
			roles = append(roles, converted)
			cache.Store(dbrole.RoleIdentifier().String(), converted)
		}
	}

	return roles, nil
}

// ConvertDBPermissions converts database permissions to RBAC permissions.
func ConvertDBPermissions(dbPerms []database.CustomRolePermission) []rbac.Permission {
	n := make([]rbac.Permission, 0, len(dbPerms))
	for _, dbPerm := range dbPerms {
		n = append(n, rbac.Permission{
			Negate:       dbPerm.Negate,
			ResourceType: dbPerm.ResourceType,
			Action:       dbPerm.Action,
		})
	}
	return n
}

// ConvertPermissionsToDB converts RBAC permissions to the database
// format.
func ConvertPermissionsToDB(perms []rbac.Permission) []database.CustomRolePermission {
	dbPerms := make([]database.CustomRolePermission, 0, len(perms))
	for _, perm := range perms {
		dbPerms = append(dbPerms, database.CustomRolePermission{
			Negate:       perm.Negate,
			ResourceType: perm.ResourceType,
			Action:       perm.Action,
		})
	}
	return dbPerms
}

// ConvertDBRole should not be used by any human facing apis. It is used
// for authz purposes.
func ConvertDBRole(dbRole database.CustomRole) (rbac.Role, error) {
	role := rbac.Role{
		Identifier:  dbRole.RoleIdentifier(),
		DisplayName: dbRole.DisplayName,
		Site:        ConvertDBPermissions(dbRole.SitePermissions),
		User:        ConvertDBPermissions(dbRole.UserPermissions),
	}

	// Org permissions only make sense if an org id is specified.
	if len(dbRole.OrgPermissions) > 0 && dbRole.OrganizationID.UUID == uuid.Nil {
		return rbac.Role{}, xerrors.Errorf("role has organization perms without an org id specified")
	}

	if dbRole.OrganizationID.UUID != uuid.Nil {
		role.ByOrgID = map[string]rbac.OrgPermissions{
			dbRole.OrganizationID.UUID.String(): {
				Org:    ConvertDBPermissions(dbRole.OrgPermissions),
				Member: ConvertDBPermissions(dbRole.MemberPermissions),
			},
		}
	}

	return role, nil
}

// ReconcileOrgMemberRoles ensures that every organization's
// org-member system role in the DB is up-to-date with permissions
// reflecting current RBAC resources and the organization's
// workspace_sharing_disabled setting. Uses PostgreSQL advisory lock
// (LockIDReconcileOrgMemberRoles) to safely handle multi-instance
// deployments. Uses set-based comparison to avoid unnecessary
// database writes when permissions haven't changed.
func ReconcileOrgMemberRoles(ctx context.Context, log slog.Logger, db database.Store) error {
	return db.InTx(func(tx database.Store) error {
		// Acquire advisory lock to prevent concurrent updates from
		// multiple coderd instances. Other instances will block here
		// until we release the lock (when this transaction commits).
		err := tx.AcquireLock(ctx, database.LockIDReconcileOrgMemberRoles)
		if err != nil {
			return xerrors.Errorf("acquire reconcile org-member roles lock: %w", err)
		}

		orgs, err := tx.GetOrganizations(ctx, database.GetOrganizationsParams{})
		if err != nil {
			return xerrors.Errorf("fetch organizations: %w", err)
		}

		customRoles, err := tx.CustomRoles(ctx, database.CustomRolesParams{
			IncludeSystemRoles: true,
		})
		if err != nil {
			return xerrors.Errorf("fetch custom roles: %w", err)
		}

		// Find org-member roles and index by organization ID for quick lookup.
		rolesByOrg := make(map[uuid.UUID]database.CustomRole)
		for _, role := range customRoles {
			if role.IsSystem && role.Name == rbac.RoleOrgMember() && role.OrganizationID.Valid {
				rolesByOrg[role.OrganizationID.UUID] = role
			}
		}

		for _, org := range orgs {
			role, exists := rolesByOrg[org.ID]
			if !exists {
				// Something is very wrong: the role should have been created by the
				// database trigger or migration. Log loudly and try creating it as
				// a last-ditch effort before giving up.
				log.Critical(ctx, "missing organization-member system role; trying to re-create",
					slog.F("organization_id", org.ID))

				if err := CreateOrgMemberRole(ctx, tx, org); err != nil {
					return xerrors.Errorf("create missing org-member role for organization %s: %w",
						org.ID, err)
				}

				// Nothing more to do; the new role's permissions are up-to-date.
				continue
			}

			// Generate expected perms based on org's workspace sharing setting.
			expectedOrgPerms, expectedMemberPerms := rbac.OrgMemberPermissions(org.WorkspaceSharingDisabled)

			storedOrgPerms := ConvertDBPermissions(role.OrgPermissions)
			storedMemberPerms := ConvertDBPermissions(role.MemberPermissions)

			// Compare using set-based comparison (order doesn't matter).
			orgPermsMatch := rbac.PermissionsEqual(expectedOrgPerms, storedOrgPerms)
			memberPermsMatch := rbac.PermissionsEqual(expectedMemberPerms, storedMemberPerms)

			if !orgPermsMatch || !memberPermsMatch {
				_, err = tx.UpdateCustomRole(ctx, database.UpdateCustomRoleParams{
					Name:              role.Name,
					OrganizationID:    role.OrganizationID,
					DisplayName:       role.DisplayName,
					SitePermissions:   role.SitePermissions,
					OrgPermissions:    ConvertPermissionsToDB(expectedOrgPerms),
					UserPermissions:   role.UserPermissions,
					MemberPermissions: ConvertPermissionsToDB(expectedMemberPerms),
				})
				if err != nil {
					return xerrors.Errorf("update org-member role for organization %s: %w",
						org.ID, err)
				}
			}
		}

		return nil
	}, nil)
}

// CreateOrgMemberRole creates the org-member system role for an organization.
func CreateOrgMemberRole(ctx context.Context, tx database.Store, org database.Organization) error {
	orgPerms, memberPerms := rbac.OrgMemberPermissions(org.WorkspaceSharingDisabled)

	_, err := tx.InsertCustomRole(ctx, database.InsertCustomRoleParams{
		Name:              rbac.RoleOrgMember(),
		DisplayName:       "",
		OrganizationID:    uuid.NullUUID{UUID: org.ID, Valid: true},
		SitePermissions:   database.CustomRolePermissions{},
		OrgPermissions:    ConvertPermissionsToDB(orgPerms),
		UserPermissions:   database.CustomRolePermissions{},
		MemberPermissions: ConvertPermissionsToDB(memberPerms),
		IsSystem:          true,
	})
	if err != nil {
		return xerrors.Errorf("insert org-member role: %w", err)
	}

	return nil
}
