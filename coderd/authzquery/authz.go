package authzquery

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"
)

// TODO:
// - We need to handle authorizing the CRUD of objects with RBAC being related
//   to some other object. Eg: workspace builds, group members, etc.

func authorizedDelete[ObjectType rbac.Objecter, ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Delete func(ctx context.Context, arg ArgumentType) error](
	// Arguments
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	deleteFunc Delete) Delete {

	return authorizedFetchAndExecWithConverter(authorizer,
		rbac.ActionDelete,
		func(o ObjectType) rbac.Object {
			return o.RBACObject()
		}, fetchFunc, deleteFunc)
}

func authorizedUpdate[ObjectType rbac.Objecter, ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Exec func(ctx context.Context, arg ArgumentType) error](
	// Arguments
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	deleteFunc Exec) Exec {

	return authorizedFetchAndExecWithConverter(authorizer,
		rbac.ActionUpdate,
		func(o ObjectType) rbac.Object {
			return o.RBACObject()
		}, fetchFunc, deleteFunc)
}

// authorizedFetchAndExecWithConverter uses authorizedFetchAndQueryWithConverter but
// only cares about the error return type. SQL execs only return an error.
// See authorizedFetchAndQueryWithConverter for more details.
func authorizedFetchAndExecWithConverter[ObjectType any, ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Exec func(ctx context.Context, arg ArgumentType) error](
	// Arguments
	authorizer rbac.Authorizer,
	action rbac.Action,
	objectToRBAC func(o ObjectType) rbac.Object,
	fetchFunc Fetch,
	execFunc Exec) Exec {

	f := authorizedFetchAndQueryWithConverter(authorizer, action, objectToRBAC, fetchFunc, func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		return empty, execFunc(ctx, arg)
	})
	return func(ctx context.Context, arg ArgumentType) error {
		_, err := f(ctx, arg)
		return err
	}
}

// authorizedFetchAndQueryWithConverter is the same as authorizedFetchAndExecWithConverter
// except it runs a query with 2 return values instead of an exec with 1 return values.
// See authorizedFetchAndExecWithConverter

// authorizedFetchAndQueryWithConverter is a generic function that wraps a database
// query function with authorization. The returned function has the same arguments
// as the database function.
//
// The function will always make a database.FetchObject before running the exec.
//
// TODO: In most cases the object is already fetched before calling the delete function.
// A method should be implemented to preload the object on the context before calling
// the delete function. This preload cache should be generic to cover more cases.
func authorizedFetchAndQueryWithConverter[ObjectType any, ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Query func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	// Arguments
	authorizer rbac.Authorizer,
	action rbac.Action,
	objectToRbac func(o ObjectType) rbac.Object,
	fetchFunc Fetch,
	queryFunc Query) Query {

	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := actorFromContext(ctx)
		if !ok {
			return empty, xerrors.Errorf("no authorization actor in context")
		}

		// Fetch the database object
		object, err := fetchFunc(ctx, arg)
		if err != nil {
			return empty, xerrors.Errorf("fetch object: %w", err)
		}

		// Authorize the action
		rbacObject := objectToRbac(object)
		err = authorizer.ByRoleName(ctx, act.ID.String(), act.Roles, act.Scope, act.Groups, action, rbacObject)
		if err != nil {
			return empty, xerrors.Errorf("unauthorized: %w", err)
		}

		return queryFunc(ctx, arg)
	}
}

func authorizedFetch[ObjectType rbac.Objecter, ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	// Arguments
	authorizer rbac.Authorizer,
	fetchFunc Fetch) Fetch {

	return authorizedQueryWithConverter(authorizer,
		func(o ObjectType) rbac.Object {
			return o.RBACObject()
		}, fetchFunc)
}

// authorizedQueryWithConverter is a generic function that wraps a database
// query function (returns an object and an error) with authorization. The
// returned function has the same arguments as the database function.
//
// The database query function will **ALWAYS** hit the database, even if the
// user cannot read the resource. This is because the resource details are
// required to run a proper authorization check.
//
// An optimized version of this could be written if the object's authz
// subject properties are known by the caller.
func authorizedQueryWithConverter[ArgumentType any, ObjectType any,
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

// authorizedFetchSet is like authorizedFetch, but works with lists of objects.
// SQL filters are much more optimal.
func authorizedFetchSet[ArgumentType any, ObjectType rbac.Objecter,
	DatabaseFunc func(ctx context.Context, arg ArgumentType) ([]ObjectType, error)](
	// Arguments
	authorizer rbac.Authorizer,
	f DatabaseFunc) DatabaseFunc {

	return func(ctx context.Context, arg ArgumentType) (empty []ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := actorFromContext(ctx)
		if !ok {
			return empty, xerrors.Errorf("no authorization actor in context")
		}

		// Fetch the database object
		objects, err := f(ctx, arg)
		if err != nil {
			return nil, xerrors.Errorf("fetch object: %w", err)
		}

		// Authorize the action
		return rbac.Filter(ctx, authorizer, act.ID.String(), act.Roles, act.Scope, act.Groups, rbac.ActionRead, objects)
	}
}

// prepareSQLFilter is a helper function that prepares a SQL filter using the
// given authorization context.
func prepareSQLFilter(ctx context.Context, authorizer rbac.Authorizer, action rbac.Action, resourceType string) (rbac.PreparedAuthorized, error) {
	act, ok := actorFromContext(ctx)
	if !ok {
		return nil, xerrors.Errorf("no authorization actor in context")
	}

	return authorizer.PrepareByRoleName(ctx, act.ID.String(), act.Roles, act.Scope, act.Groups, action, resourceType)
}
