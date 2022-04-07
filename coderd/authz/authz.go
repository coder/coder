package authz

import (
	"errors"

	"github.com/coder/coder/coderd/authz/rbac"
)

var ErrUnauthorized = errors.New("unauthorized")

// TODO: Implement Authorize
func Authorize(subj Subject, res Object, action rbac.Operation) error {
	// TODO: Expand subject roles into their permissions as appropriate. Apply scopes.

	if res.ObjectType == "" {
		return ErrUnauthorized
	}

	if SiteEnforcer.RolesHavePermission(subj.Roles(), res.ObjectType, action) {
		return nil
	}

	return ErrUnauthorized
}
