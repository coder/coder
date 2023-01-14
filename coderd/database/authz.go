package database

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/rbac"
)

type authContextKey struct{}

type actor struct {
	ID     uuid.UUID
	Roles  []string
	Scope  rbac.Scope
	Groups []string
}

func WithAuthorizeContext(ctx context.Context, actorID uuid.UUID, roles []string, groups []string, scope rbac.Scope) context.Context {
	return context.WithValue(ctx, authContextKey{}, actor{
		ID:     actorID,
		Roles:  roles,
		Scope:  scope,
		Groups: groups,
	})
}

func actorFromContext(ctx context.Context) (actor, bool) {
	a, ok := ctx.Value(authContextKey{}).(actor)
	return a, ok
}

type AuthzQuerier struct {
	database   Store
	authorizer rbac.Authorizer
}

func NewAuthzQuerier(db Store, authorizer rbac.Authorizer) *AuthzQuerier {
	return &AuthzQuerier{
		database:   db,
		authorizer: authorizer,
	}
}

func (q *AuthzQuerier) Ping(ctx context.Context) (time.Duration, error) {
	return q.database.Ping(ctx)
}

//func (q *AuthzQuerier) InTx(function func(Store) error, txOpts *sql.TxOptions) error {
//	return q.database.InTx(func(tx Store) error {
//		// Wrap the transaction store in an AuthzQuerier.
//		wrapped := NewAuthzQuerier(tx, q.authorizer)
//		return function(wrapped)
//	}, txOpts)
//}

func authorizedFetch[ArgumentType any, ObjectType rbac.Objecter, DatabaseFunc func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	authorizer rbac.Authorizer, action rbac.Action, f DatabaseFunc) DatabaseFunc {
	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		act, ok := actorFromContext(ctx)
		if !ok {
			return empty, xerrors.Errorf("no authorization actor in context")
		}

		object, err := f(ctx, arg)
		if err != nil {
			return empty, err
		}

		err = authorizer.ByRoleName(ctx, act.ID.String(), act.Roles, act.Scope, act.Groups, action, object.RBACObject())
		if err != nil {
			return empty, xerrors.Errorf("unauthorized: %w", err)
		}

		return object, nil
	}
}
