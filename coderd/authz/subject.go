package authz

import (
	"context"

	"github.com/coder/coder/coderd/authz/rbac"
)

// Subject is the actor that is attempting to do some action on some object or
// set of objects.
type Subject interface {
	// ID is the ID for the given actor. If it matches the OwnerID ID of the
	// object, we can assume the object is owned by this subject.
	ID() string

	Roles() (rbac.Roles, error)

	// OrgRoles only need to be returned for the organization in question.
	// This is because users typically belong to more than 1 organization,
	// and grabbing all the roles for all orgs is excessive.
	OrgRoles(ctx context.Context, ownerID string) (rbac.Roles, error)

	//// Scopes can limit the roles above.
	//Scopes() ([]Permission, error)
}

// SubjectTODO is a placeholder until we get an actual actor struct in place.
// This will come with the Authn epic.
// TODO: @emyrk delete this data structure when authn exists
type SubjectTODO struct {
	UserID string `json:"user_id"`

	Site rbac.Roles            `json:"site_roles"`
	Org  map[string]rbac.Roles `json:"org_roles"`
}

func (s SubjectTODO) ID() string {
	return s.UserID
}

func (s SubjectTODO) Roles() (rbac.Roles, error) {
	return s.Site, nil
}

func (s SubjectTODO) OrgRoles(_ context.Context, orgID string) (rbac.Roles, error) {
	v, ok := s.Org[orgID]
	if !ok {
		// Members not in an org return the negative perm
		return rbac.Roles{}, nil
	}

	return v, nil
}
