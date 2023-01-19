package authzquery

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"
)

// authorizedFetch is a generic function that wraps a database fetch function
// with authorization. The returned function has the same arguments as the database
// function.
//
// The database fetch function will **ALWAYS** hit the database, even if the
// user cannot read the resource. This is because the resource details are
// required to run a proper authorization check.
//
// An optimized version of this could be written if the object's authz
// subject properties are known by the caller.
func authorizedFetch[ArgumentType any, ObjectType rbac.Objecter, DatabaseFunc func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	authorizer rbac.Authorizer, action rbac.Action, f DatabaseFunc) DatabaseFunc {
	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := actorFromContext(ctx)
		if !ok {
			return empty, xerrors.Errorf("no authorization actor in context")
		}

		// Fetch the database object
		object, err := f(ctx, arg)
		if err != nil {
			return empty, err
		}

		// Authorize the action
		err = authorizer.ByRoleName(ctx, act.ID.String(), act.Roles, act.Scope, act.Groups, action, object.RBACObject())
		if err != nil {
			return empty, xerrors.Errorf("unauthorized: %w", err)
		}

		return object, nil
	}
}
