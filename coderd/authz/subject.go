package authz

import "context"

type Subject interface {
	ID() string

	SiteRoles() ([]Role, error)
	OrgRoles(ctx context.Context, orgID string) ([]Role, error)
	UserRoles() ([]Role, error)

	//Scopes() ([]Permission, error)
}

type SimpleSubject struct {
	UserID string `json:"user_id"`

	Site []Role `json:"site_roles"`
	Org  []Role `json:"org_roles"`
	User []Role `json:"user_roles"`
}

func (s SimpleSubject) ID() string {
	return s.UserID
}

func (s SimpleSubject) SiteRoles() ([]Role, error) {
	return s.Site, nil
}

func (s SimpleSubject) OrgRoles() ([]Role, error) {
	return s.Org, nil
}

func (s SimpleSubject) UserRoles() ([]Role, error) {
	return s.User, nil
}
