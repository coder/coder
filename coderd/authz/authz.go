package authz

import "errors"

var ErrUnauthorized = errors.New("unauthorized")

// TODO: Implement Authorize
func Authorize(subj Subject, obj Resource, action Action) error {
	// TODO: Expand subject roles into their permissions as appropriate. Apply scopes.
	return AuthorizePermissions(subj.ID(), []Permission{}, obj, action)
}

// AuthorizePermissions runs the authorize function with the raw permissions in a single list.
func AuthorizePermissions(_ string, _ []Permission, _ Resource, _ Action) error {
	// return nil
	// for now, nothing is allowed
	return ErrUnauthorized
}
