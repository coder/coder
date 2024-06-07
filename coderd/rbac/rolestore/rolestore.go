package rolestore

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

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
	return context.WithValue(ctx, customRoleCtxKey{}, syncmap.New[rbac.UniqueRoleName, rbac.Role]())
}

func roleCache(ctx context.Context) *syncmap.Map[rbac.UniqueRoleName, rbac.Role] {
	c, ok := ctx.Value(customRoleCtxKey{}).(*syncmap.Map[rbac.UniqueRoleName, rbac.Role])
	if !ok {
		return syncmap.New[rbac.UniqueRoleName, rbac.Role]()
	}
	return c
}

// Expand will expand built in roles, and fetch custom roles from the database.
func Expand(ctx context.Context, db database.Store, names []rbac.UniqueRoleName) (rbac.Roles, error) {
	if len(names) == 0 {
		// That was easy
		return []rbac.Role{}, nil
	}

	cache := roleCache(ctx)
	lookup := make([]rbac.UniqueRoleName, 0)
	roles := make([]rbac.Role, 0, len(names))

	for _, name := range names {
		// Remove any built in roles
		expanded, err := rbac.RoleByName(name)
		if err == nil {
			roles = append(roles, expanded)
			continue
		}

		// Check custom role cache
		customRole, ok := cache.Load(name)
		if ok {
			roles = append(roles, customRole)
			continue
		}

		// Defer custom role lookup
		lookup = append(lookup, name)
	}

	if len(lookup) > 0 {
		// The set of roles coming in are formatted as 'rolename[:<org_id>]'.
		// In the database, org roles are scoped with an organization column.
		lookupArgs := make([]database.NameOrganizationPair, 0, len(lookup))
		for _, name := range lookup {
			roleName, orgID, err := name.Split()
			if err != nil {
				continue
			}

			parsedOrgID := uuid.Nil // Default to no org ID
			if orgID != "" {
				parsedOrgID, err = uuid.Parse(orgID)
				if err != nil {
					continue
				}
			}

			lookupArgs = append(lookupArgs, database.NameOrganizationPair{
				Name:           roleName,
				OrganizationID: parsedOrgID,
			})
		}

		// If some roles are missing from the database, they are omitted from
		// the expansion. These roles are no-ops. Should we raise some kind of
		// warning when this happens?
		dbroles, err := db.CustomRoles(ctx, database.CustomRolesParams{
			LookupRoles:     lookupArgs,
			ExcludeOrgRoles: false,
			OrganizationID:  uuid.Nil,
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
			cache.Store(dbrole.UniqueName(), converted)
		}
	}

	return roles, nil
}

func convertPermissions(dbPerms []database.CustomRolePermission) []rbac.Permission {
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

// ConvertDBRole should not be used by any human facing apis. It is used
// for authz purposes.
func ConvertDBRole(dbRole database.CustomRole) (rbac.Role, error) {
	role := rbac.Role{
		Name:        dbRole.UniqueName(),
		DisplayName: dbRole.DisplayName,
		Site:        convertPermissions(dbRole.SitePermissions),
		Org:         nil,
		User:        convertPermissions(dbRole.UserPermissions),
	}

	// Org permissions only make sense if an org id is specified.
	if len(dbRole.OrgPermissions) > 0 && dbRole.OrganizationID.UUID == uuid.Nil {
		return rbac.Role{}, xerrors.Errorf("role has organization perms without an org id specified")
	}

	if dbRole.OrganizationID.UUID != uuid.Nil {
		role.Org = map[string][]rbac.Permission{
			dbRole.OrganizationID.UUID.String(): convertPermissions(dbRole.OrgPermissions),
		}
	}

	return role, nil
}
