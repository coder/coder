package authz

import "github.com/coder/coder/coderd/authz/rbac"

// TODO: Implement Authorize
func Authorize(subj Subject, obj Resource, action rbac.Operation) error {
	// TODO: Expand subject roles into their permissions as appropriate. Apply scopes.

	return nil
}
