package rolestore

import (
	"context"
	"maps"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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

// System roles are defined in code but stored in the database,
// allowing their permissions to be adjusted per-organization at
// runtime based on org settings (e.g. workspace sharing).
var systemRoles = map[string]permissionsFunc{
	rbac.RoleOrgMember():         rbac.OrgMemberPermissions,
	rbac.RoleOrgServiceAccount(): rbac.OrgServiceAccountPermissions,
}

// permissionsFunc produces the desired permissions for a system role
// given organization settings.
type permissionsFunc func(rbac.OrgSettings) rbac.OrgRolePermissions

func IsSystemRoleName(name string) bool {
	_, ok := systemRoles[name]
	return ok
}

var SystemRoleNames = maps.Keys(systemRoles)

// ReconcileSystemRoles ensures that every organization's system roles
// in the DB are up-to-date with the current RBAC definitions and
// organization settings.
func ReconcileSystemRoles(ctx context.Context, log slog.Logger, db database.Store) error {
	return db.InTx(func(tx database.Store) error {
		// Acquire advisory lock to prevent concurrent updates from
		// multiple coderd instances. Other instances will block here
		// until we release the lock (when this transaction commits).
		err := tx.AcquireLock(ctx, database.LockIDReconcileSystemRoles)
		if err != nil {
			return xerrors.Errorf("acquire system roles reconciliation lock: %w", err)
		}

		orgs, err := tx.GetOrganizations(ctx, database.GetOrganizationsParams{})
		if err != nil {
			return xerrors.Errorf("fetch organizations: %w", err)
		}

		customRoles, err := tx.CustomRoles(ctx, database.CustomRolesParams{
			LookupRoles:        nil,
			ExcludeOrgRoles:    false,
			OrganizationID:     uuid.Nil,
			IncludeSystemRoles: true,
		})
		if err != nil {
			return xerrors.Errorf("fetch custom roles: %w", err)
		}

		// Index system roles by (org ID, role name) for quick lookup.
		type orgRoleKey struct {
			OrgID    uuid.UUID
			RoleName string
		}
		roleIndex := make(map[orgRoleKey]database.CustomRole)
		for _, role := range customRoles {
			if role.IsSystem && IsSystemRoleName(role.Name) && role.OrganizationID.Valid {
				roleIndex[orgRoleKey{role.OrganizationID.UUID, role.Name}] = role
			}
		}

		for _, org := range orgs {
			for roleName := range systemRoles {
				role, exists := roleIndex[orgRoleKey{org.ID, roleName}]
				if !exists {
					// Something is very wrong: the role should have been
					// created by the db trigger or migration. Log loudly and
					// try creating it as a last-ditch effort before giving up.
					log.Critical(ctx, "missing system role; trying to re-create",
						slog.F("organization_id", org.ID),
						slog.F("role_name", roleName))

					err := CreateSystemRole(ctx, tx, org, roleName)
					if err != nil {
						return xerrors.Errorf("create missing %s system role for organization %s: %w",
							roleName, org.ID, err)
					}

					// Nothing more to do; the new role's permissions are
					// up-to-date.
					continue
				}

				_, _, err := ReconcileSystemRole(ctx, tx, role, org)
				if err != nil {
					return xerrors.Errorf("reconcile %s system role for organization %s: %w",
						roleName, org.ID, err)
				}
			}
		}

		return nil
	}, nil)
}

// ReconcileSystemRole compares the given role's permissions against
// the desired permissions produced by the permissions function based
// on the organization's settings. If they differ, the DB row is
// updated. Uses set-based comparison so permission ordering doesn't
// matter. Returns the correct role and a boolean indicating whether
// the reconciliation was necessary.
//
// IMPORTANT: Callers must hold database.LockIDReconcileSystemRoles
// for the duration of the enclosing transaction.
func ReconcileSystemRole(
	ctx context.Context,
	tx database.Store,
	in database.CustomRole,
	org database.Organization,
) (database.CustomRole, bool, error) {
	permsFunc, ok := systemRoles[in.Name]
	if !ok {
		panic("dev error: no permissions function exists for role " + in.Name)
	}

	// All fields except OrgPermissions and MemberPermissions will be the same.
	out := in

	// Paranoia check: we don't use these in custom roles yet.
	out.SitePermissions = database.CustomRolePermissions{}
	out.UserPermissions = database.CustomRolePermissions{}
	out.DisplayName = ""

	inOrgPerms := ConvertDBPermissions(in.OrgPermissions)
	inMemberPerms := ConvertDBPermissions(in.MemberPermissions)

	outPerms := permsFunc(orgSettings(org))

	match := rbac.PermissionsEqual(inOrgPerms, outPerms.Org) &&
		rbac.PermissionsEqual(inMemberPerms, outPerms.Member)

	if !match {
		out.OrgPermissions = ConvertPermissionsToDB(outPerms.Org)
		out.MemberPermissions = ConvertPermissionsToDB(outPerms.Member)

		_, err := tx.UpdateCustomRole(ctx, database.UpdateCustomRoleParams{
			Name:              out.Name,
			OrganizationID:    out.OrganizationID,
			DisplayName:       out.DisplayName,
			SitePermissions:   out.SitePermissions,
			UserPermissions:   out.UserPermissions,
			OrgPermissions:    out.OrgPermissions,
			MemberPermissions: out.MemberPermissions,
		})
		if err != nil {
			return out, !match, xerrors.Errorf("update %s system role for organization %s: %w",
				in.Name, in.OrganizationID.UUID, err)
		}
	}

	return out, !match, nil
}

// orgSettings maps database.Organization fields to the
// rbac.OrgSettings struct, bridging the database and rbac packages
// without introducing a circular dependency.
func orgSettings(org database.Organization) rbac.OrgSettings {
	return rbac.OrgSettings{
		ShareableWorkspaceOwners: rbac.ShareableWorkspaceOwners(org.ShareableWorkspaceOwners),
	}
}

// CreateSystemRole inserts a new system role into the database with
// permissions produced by permsFunc based on the organization's current
// settings.
func CreateSystemRole(
	ctx context.Context,
	tx database.Store,
	org database.Organization,
	roleName string,
) error {
	permsFunc, ok := systemRoles[roleName]
	if !ok {
		panic("dev error: no permissions function exists for role " + roleName)
	}
	perms := permsFunc(orgSettings(org))

	_, err := tx.InsertCustomRole(ctx, database.InsertCustomRoleParams{
		Name:              roleName,
		DisplayName:       "",
		OrganizationID:    uuid.NullUUID{UUID: org.ID, Valid: true},
		SitePermissions:   database.CustomRolePermissions{},
		OrgPermissions:    ConvertPermissionsToDB(perms.Org),
		UserPermissions:   database.CustomRolePermissions{},
		MemberPermissions: ConvertPermissionsToDB(perms.Member),
		IsSystem:          true,
	})
	if err != nil {
		return xerrors.Errorf("insert %s role: %w", roleName, err)
	}

	return nil
}
