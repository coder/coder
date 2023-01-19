package authzquery

import (
	"context"

	"github.com/coder/coder/coderd/rbac"
	"github.com/google/uuid"
)

type authContextKey struct{}

// actor is the authorization subject for a request.
// This is **required** for all AuthzQuerier operations.
type actor struct {
	ID     uuid.UUID
	Roles  []string
	Scope  rbac.ScopeName
	Groups []string
}

func WithAuthorizeContext(ctx context.Context, actorID uuid.UUID, roles []string, groups []string, scope rbac.ScopeName) context.Context {
	return context.WithValue(ctx, authContextKey{}, actor{
		ID:     actorID,
		Roles:  roles,
		Scope:  scope,
		Groups: groups,
	})
}

// WithWorkspaceAgentTokenContext returns a context with a workspace agent token
// authorization subject. A workspace agent authorization subject is the
// workspace owner's authorization subject + a workspace agent scope.
//
// TODO: The arguments and usage of this function are not finalized. It might
// be a bit awkward to use at present. The arguments are required to build the
// required authorization context. The arguments should be the owner of the
// workspace authorization roles.
func WithWorkspaceAgentTokenContext(ctx context.Context, workspaceID uuid.UUID, actorID uuid.UUID, roles []string, groups []string) context.Context {
	// TODO: This workspace ID should be applied in the scope.
	var _ = workspaceID
	return context.WithValue(ctx, authContextKey{}, actor{
		ID:    actorID,
		Roles: roles,
		// TODO: @emyrk This scope is INCORRECT. The correct scope is a readonly
		// scope for the specified workspaceID. Limit the permissions as much as
		// possible. This is a temporary scope until the scope allow_list
		// functionality exists.
		Scope:  rbac.ScopeAll,
		Groups: groups,
	})
}

// actorFromContext returns the authorization subject from the context.
// All authentication flows should set the authorization subject in the context.
// If no actor is present, the function returns false.
func actorFromContext(ctx context.Context) (actor, bool) {
	a, ok := ctx.Value(authContextKey{}).(actor)
	return a, ok
}
