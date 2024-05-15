package storedroles

import (
	"context"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/rbac"
)

type StoredRoles struct {
	db database.Store
}

func New(db database.Store) *StoredRoles {
	return &StoredRoles{
		db: db,
	}
}

type Roles struct {
	sr    StoredRoles
	names []string

	// Compute once, then serve the cached result.
	cached      []rbac.Role
	cachedError error
}

// ExpandableRoles returns a struct that expand role names into their rbac.Roles.
// Do not expand at this call time, instead expand lazy when `Expand()` is called.
func (sr StoredRoles) ExpandableRoles(names []string) *Roles {
	return &Roles{
		sr:    sr,
		names: names,
	}
}

// Expand will try to expand built-ins, then it's local cache, then
// it will go the database.
func (r Roles) Expand(ctx context.Context) ([]rbac.Role, error) {
	if len(r.names) == 0 {
		// That was easy
		return []rbac.Role{}, nil
	}

	// Use the cache first.
	if len(r.cached) != 0 || r.cachedError != nil {
		return r.cached, r.cachedError
	}

	lookup := make([]string, 0)
	roles := make([]rbac.Role, 0, len(r.names))

	for _, name := range r.names {
		// Remove any built in roles
		expanded, err := rbac.RoleByName(name)
		if err == nil {
			roles = append(roles, expanded)
			continue
		}

		// Defer custom role lookup
		lookup = append(lookup, name)
	}

	if len(lookup) > 0 {
		r.sr.db.CustomRoles(ctx, lookup)
	}

	return roles, nil
}

func (r Roles) Names() []string {
	return r.names
}
