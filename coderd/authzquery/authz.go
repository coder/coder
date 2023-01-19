package authzquery

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"
)

func authorizedDelete[ObjectType rbac.Objecter, ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Delete func(ctx context.Context, arg ArgumentType) error](
	// Arguments
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	deleteFunc Delete) Delete {

	return authorizedDeleteWithConverter(authorizer,
		func(o ObjectType) rbac.Object {
			return o.RBACObject()
		}, fetchFunc, deleteFunc)
}

// authorizedDeleteWithConverter is a generic function that wraps a database delete function
// with authorization. The returned function has the same arguments as the database
// function.
//
// The function will always make a database.FetchObject before deleting the object.
//
// TODO: In most cases the object is already fetched before calling the delete function.
// A method should be implemented to preload the object on the context before calling
// the delete function. This preload cache should be generic to cover more cases.
func authorizedDeleteWithConverter[ObjectType any, ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Delete func(ctx context.Context, arg ArgumentType) error](
	// Arguments
	authorizer rbac.Authorizer,
	objectToRbac func(o ObjectType) rbac.Object,
	fetchFunc Fetch,
	deleteFunc Delete) Delete {

	return func(ctx context.Context, arg ArgumentType) (err error) {
		// Fetch the rbac subject
		act, ok := actorFromContext(ctx)
		if !ok {
			return xerrors.Errorf("no authorization actor in context")
		}

		// Fetch the database object
		object, err := fetchFunc(ctx, arg)
		if err != nil {
			return xerrors.Errorf("fetch object: %w", err)
		}

		// Authorize the action
		rbacObject := objectToRbac(object)
		err = authorizer.ByRoleName(ctx, act.ID.String(), act.Roles, act.Scope, act.Groups, rbac.ActionDelete, rbacObject)
		if err != nil {
			return xerrors.Errorf("unauthorized: %w", err)
		}

		return deleteFunc(ctx, arg)
	}
}

func authorizedFetch[ObjectType rbac.Objecter, ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	// Arguments
	authorizer rbac.Authorizer,
	fetchFunc Fetch) Fetch {

	return authorizedFetchWithConverter(authorizer,
		func(o ObjectType) rbac.Object {
			return o.RBACObject()
		}, fetchFunc)
}

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
func authorizedFetchWithConverter[ArgumentType any, ObjectType any,
	DatabaseFunc func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	// Arguments
	authorizer rbac.Authorizer,
	objectToRbac func(o ObjectType) rbac.Object,
	f DatabaseFunc) DatabaseFunc {

	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := actorFromContext(ctx)
		if !ok {
			return empty, xerrors.Errorf("no authorization actor in context")
		}

		// Fetch the database object
		object, err := f(ctx, arg)
		if err != nil {
			return empty, xerrors.Errorf("fetch object: %w", err)
		}

		// Authorize the action
		rbacObject := objectToRbac(object)
		err = authorizer.ByRoleName(ctx, act.ID.String(), act.Roles, act.Scope, act.Groups, rbac.ActionRead, rbacObject)
		if err != nil {
			return empty, xerrors.Errorf("unauthorized: %w", err)
		}

		return object, nil
	}
}
