package authzquery

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"
)

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
