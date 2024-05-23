package rolestore

import (
	"context"
	"encoding/json"
	"net/http"

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
func Expand(ctx context.Context, db database.Store, names []string) (rbac.Roles, error) {
	if len(names) == 0 {
		// That was easy
		return []rbac.Role{}, nil
	}

	cache := roleCache(ctx)
	lookup := make([]string, 0)
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
		// If some roles are missing from the database, they are omitted from
		// the expansion. These roles are no-ops. Should we raise some kind of
		// warning when this happens?
		dbroles, err := db.CustomRoles(ctx, database.CustomRolesParams{
			LookupRoles:     lookup,
			ExcludeOrgRoles: false,
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
			cache.Store(dbrole.Name, converted)
		}
	}

	return roles, nil
}

func ConvertDBRole(dbRole database.CustomRole) (rbac.Role, error) {
	role := rbac.Role{
		Name:        dbRole.Name,
		DisplayName: dbRole.DisplayName,
		Site:        nil,
		Org:         nil,
		User:        nil,
	}

	err := json.Unmarshal(dbRole.SitePermissions, &role.Site)
	if err != nil {
		return role, xerrors.Errorf("unmarshal site permissions: %w", err)
	}

	err = json.Unmarshal(dbRole.OrgPermissions, &role.Org)
	if err != nil {
		return role, xerrors.Errorf("unmarshal org permissions: %w", err)
	}

	err = json.Unmarshal(dbRole.UserPermissions, &role.User)
	if err != nil {
		return role, xerrors.Errorf("unmarshal user permissions: %w", err)
	}

	return role, nil
}

func ConvertRoleToDB(role rbac.Role) (database.CustomRole, error) {
	dbRole := database.CustomRole{
		Name:        role.Name,
		DisplayName: role.DisplayName,
	}

	siteData, err := json.Marshal(role.Site)
	if err != nil {
		return dbRole, xerrors.Errorf("marshal site permissions: %w", err)
	}
	dbRole.SitePermissions = siteData

	orgData, err := json.Marshal(role.Org)
	if err != nil {
		return dbRole, xerrors.Errorf("marshal org permissions: %w", err)
	}
	dbRole.OrgPermissions = orgData

	userData, err := json.Marshal(role.User)
	if err != nil {
		return dbRole, xerrors.Errorf("marshal user permissions: %w", err)
	}
	dbRole.UserPermissions = userData

	return dbRole, nil
}
