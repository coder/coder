package authzquery

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"
)

// TODO:
// - We need to handle authorizing the CRUD of objects with RBAC being related
//   to some other object. Eg: workspace builds, group members, etc.

func authorizedInsert[ArgumentType any,
	Insert func(ctx context.Context, arg ArgumentType) error](
	// Arguments
	authorizer rbac.Authorizer,
	action rbac.Action,
	object rbac.Objecter,
	insertFunc Insert) Insert {

	return func(ctx context.Context, arg ArgumentType) error {
		_, err := authorizedInsertWithReturn(authorizer, action, object, func(ctx context.Context, arg ArgumentType) (rbac.Objecter, error) {
			return rbac.Object{}, insertFunc(ctx, arg)
		})(ctx, arg)
		return err
	}
}

func authorizedInsertWithReturn[ObjectType any, ArgumentType any,
	Insert func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	// Arguments
	authorizer rbac.Authorizer,
	action rbac.Action,
	object rbac.Objecter,
	insertFunc Insert) Insert {

	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := actorFromContext(ctx)
		if !ok {
			return empty, xerrors.Errorf("no authorization actor in context")
		}

		// Authorize the action
		err = authorizer.Authorize(ctx, act, action, object.RBACObject())
		if err != nil {
			return empty, xerrors.Errorf("unauthorized: %w", err)
		}

		// Insert the database object
		return insertFunc(ctx, arg)
	}
}

func authorizedDelete[ObjectType rbac.Objecter, ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Delete func(ctx context.Context, arg ArgumentType) error](
	// Arguments
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	deleteFunc Delete) Delete {

	return authorizedFetchAndExec(authorizer,
		rbac.ActionDelete, fetchFunc, deleteFunc)
}

func authorizedUpdateWithReturn[ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	UpdateQuery func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	// Arguments
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	updateQuery UpdateQuery) UpdateQuery {

	return authorizedFetchAndQuery(authorizer, rbac.ActionUpdate, fetchFunc, updateQuery)
}

func authorizedUpdate[ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Exec func(ctx context.Context, arg ArgumentType) error](
	// Arguments
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	updateExec Exec) Exec {

	return authorizedFetchAndExec(authorizer, rbac.ActionUpdate, fetchFunc, updateExec)
}

// authorizedFetchAndExecWithConverter uses authorizedFetchAndQueryWithConverter but
// only cares about the error return type. SQL execs only return an error.
// See authorizedFetchAndQueryWithConverter for more details.
func authorizedFetchAndExec[ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Exec func(ctx context.Context, arg ArgumentType) error](
	// Arguments
	authorizer rbac.Authorizer,
	action rbac.Action,
	fetchFunc Fetch,
	execFunc Exec) Exec {

	f := authorizedFetchAndQuery(authorizer, action, fetchFunc, func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		return empty, execFunc(ctx, arg)
	})
	return func(ctx context.Context, arg ArgumentType) error {
		_, err := f(ctx, arg)
		return err
	}
}

func authorizedFetchAndQuery[ObjectType rbac.Objecter, ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Query func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	// Arguments
	authorizer rbac.Authorizer,
	action rbac.Action,
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
		err = authorizer.Authorize(ctx, act, action, object.RBACObject())
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

	return authorizedQuery(authorizer, rbac.ActionRead, fetchFunc)
}

// authorizedQuery is a generic function that wraps a database
// query function (returns an object and an error) with authorization. The
// returned function has the same arguments as the database function.
//
// The database query function will **ALWAYS** hit the database, even if the
// user cannot read the resource. This is because the resource details are
// required to run a proper authorization check.
//
// An optimized version of this could be written if the object's authz
// subject properties are known by the caller.
func authorizedQuery[ArgumentType any, ObjectType rbac.Objecter,
	DatabaseFunc func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	// Arguments
	authorizer rbac.Authorizer,
	action rbac.Action,
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
		err = authorizer.Authorize(ctx, act, action, object.RBACObject())
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
		return rbac.Filter(ctx, authorizer, act, rbac.ActionRead, objects)
	}
}

// authorizedQueryWithRelated performs the same function as authorizedQuery, except that
// RBAC checks are performed on the result of relatedFunc() instead of the result of fetch().
// This is useful for cases where ObjectType does not implement RBACObjecter.
// For example, a TemplateVersion object does not implement RBACObjecter, but it is
// related to a Template object, which does. Thus, any operations on a TemplateVersion
// are predicated on the RBAC permissions of the related Template object.
func authorizedQueryWithRelated[ObjectType any, ArgumentType any, Related rbac.Objecter](
	// Arguments
	authorizer rbac.Authorizer,
	action rbac.Action,
	relatedFunc func(ObjectType, ArgumentType) (Related, error),
	fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error)) func(ctx context.Context, arg ArgumentType) (ObjectType, error) {

	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := actorFromContext(ctx)
		if !ok {
			return empty, xerrors.Errorf("no authorization actor in context")
		}

		// Fetch the rbac object
		obj, err := fetch(ctx, arg)
		if err != nil {
			return empty, xerrors.Errorf("fetch object: %w", err)
		}

		// Fetch the related object on which we actually do RBAC
		rel, err := relatedFunc(obj, arg)
		if err != nil {
			return empty, xerrors.Errorf("fetch related object: %w", err)
		}

		// Authorize the action
		err = authorizer.Authorize(ctx, act, action, rel.RBACObject())
		if err != nil {
			return empty, xerrors.Errorf("unauthorized: %w", err)
		}

		return obj, nil
	}
}

// prepareSQLFilter is a helper function that prepares a SQL filter using the
// given authorization context.
func prepareSQLFilter(ctx context.Context, authorizer rbac.Authorizer, action rbac.Action, resourceType string) (rbac.PreparedAuthorized, error) {
	act, ok := actorFromContext(ctx)
	if !ok {
		return nil, xerrors.Errorf("no authorization actor in context")
	}

	return authorizer.Prepare(ctx, act, action, resourceType)
}
