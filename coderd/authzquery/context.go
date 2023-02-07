package authzquery

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/rbac"
)

// TODO:
//	- We still need a system user for system functions that a user should
//	not be able to call.

type authContextKey struct{}

func WithAuthorizeSystemContext(ctx context.Context, roles rbac.ExpandableRoles) context.Context {
	// TODO: Add protections to search for user roles. If user roles are found,
	// this should panic. That is a developer error that should be caught
	// in unit tests.
	return context.WithValue(ctx, authContextKey{}, rbac.Subject{
		ID:     uuid.Nil.String(),
		Roles:  roles,
		Scope:  rbac.ScopeAll,
		Groups: []string{},
	})
}

func WithAuthorizeContext(ctx context.Context, actor rbac.Subject) context.Context {
	return context.WithValue(ctx, authContextKey{}, actor)
}

// ActorFromContext returns the authorization subject from the context.
// All authentication flows should set the authorization subject in the context.
// If no actor is present, the function returns false.
func ActorFromContext(ctx context.Context) (rbac.Subject, bool) {
	a, ok := ctx.Value(authContextKey{}).(rbac.Subject)
	return a, ok
}
