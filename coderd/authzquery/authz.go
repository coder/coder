package authzquery

import (
	"context"
	"database/sql"
	"fmt"

	"cdr.dev/slog"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/rbac"
)

// TODO:
// - We need to handle authorizing the CRUD of objects with RBAC being related
//   to some other object. Eg: workspace builds, group members, etc.

var (
	// NoActorError wraps ErrNoRows for the api to return a 404. This is the correct
	// response when the user is not authorized.
	NoActorError = xerrors.Errorf("no authorization actor in context: %w", sql.ErrNoRows)
)

// NotAuthorizedError is a sentinel error that unwraps to sql.ErrNoRows.
// This allows the internal error to be read by the caller if needed. Otherwise
// it will be handled as a 404.
type NotAuthorizedError struct {
	Err error
}

func (e NotAuthorizedError) Error() string {
	return fmt.Sprintf("unauthorized: %s", e.Err.Error())
}

// Unwrap will always unwrap to a sql.ErrNoRows so the API returns a 404.
// So 'errors.Is(err, sql.ErrNoRows)' will always be true.
func (NotAuthorizedError) Unwrap() error {
	return sql.ErrNoRows
}

func LogNotAuthorizedError(ctx context.Context, logger slog.Logger, err error) error {
	// Only log the errors if it is an UnauthorizedError error.
	internalError := new(rbac.UnauthorizedError)
	if err != nil && xerrors.As(err, internalError) {
		logger.Debug(ctx, "unauthorized",
			slog.F("internal", internalError.Internal()),
			slog.F("input", internalError.Input()),
			slog.Error(err),
		)
	}
	return NotAuthorizedError{
		Err: err,
	}
}

// insert is the same as insertWithReturn, but does not return the inserted object.
func insert[ArgumentType any,
	Insert func(ctx context.Context, arg ArgumentType) error](
	// Arguments
	logger slog.Logger,
	authorizer rbac.Authorizer,
	object rbac.Objecter,
	insertFunc Insert) Insert {
	return func(ctx context.Context, arg ArgumentType) error {
		_, err := insertWithReturn(logger, authorizer, object, func(ctx context.Context, arg ArgumentType) (rbac.Objecter, error) {
			return rbac.Object{}, insertFunc(ctx, arg)
		})(ctx, arg)
		return err
	}
}

// insertWithReturn runs an rbac.ActionCreate on the rbac object argument before
// running the insertFunc. The insertFunc is expected to return the object that
// was inserted.
func insertWithReturn[ObjectType any, ArgumentType any,
	Insert func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	// Arguments
	logger slog.Logger,
	authorizer rbac.Authorizer,
	object rbac.Objecter,
	insertFunc Insert) Insert {
	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := ActorFromContext(ctx)
		if !ok {
			return empty, NoActorError
		}

		// Authorize the action
		err = authorizer.Authorize(ctx, act, rbac.ActionCreate, object.RBACObject())
		if err != nil {
			return empty, LogNotAuthorizedError(ctx, logger, err)
		}

		// Insert the database object
		return insertFunc(ctx, arg)
	}
}

func deleteQ[ObjectType rbac.Objecter, ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Delete func(ctx context.Context, arg ArgumentType) error](
	// Arguments
	logger slog.Logger,
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	deleteFunc Delete) Delete {
	return fetchAndExec(logger, authorizer,
		rbac.ActionDelete, fetchFunc, deleteFunc)
}

func updateWithReturn[ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	UpdateQuery func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	// Arguments
	logger slog.Logger,
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	updateQuery UpdateQuery) UpdateQuery {
	return fetchAndQuery(logger, authorizer, rbac.ActionUpdate, fetchFunc, updateQuery)
}

func update[ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Exec func(ctx context.Context, arg ArgumentType) error](
	// Arguments
	logger slog.Logger,
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	updateExec Exec) Exec {
	return fetchAndExec(logger, authorizer, rbac.ActionUpdate, fetchFunc, updateExec)
}

// fetch is a generic function that wraps a database
// query function (returns an object and an error) with authorization. The
// returned function has the same arguments as the database function.
//
// The database query function will **ALWAYS** hit the database, even if the
// user cannot read the resource. This is because the resource details are
// required to run a proper authorization check.
func fetch[ArgumentType any, ObjectType rbac.Objecter,
	DatabaseFunc func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	// Arguments
	logger slog.Logger,
	authorizer rbac.Authorizer,
	f DatabaseFunc) DatabaseFunc {
	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := ActorFromContext(ctx)
		if !ok {
			return empty, NoActorError
		}

		// Fetch the database object
		object, err := f(ctx, arg)
		if err != nil {
			return empty, xerrors.Errorf("fetch object: %w", err)
		}

		// Authorize the action
		err = authorizer.Authorize(ctx, act, rbac.ActionRead, object.RBACObject())
		if err != nil {
			return empty, LogNotAuthorizedError(ctx, logger, err)
		}

		return object, nil
	}
}

// fetchAndExec uses fetchAndQuery but only returns the error. The naming comes
// from SQL 'exec' functions which only return an error.
// See fetchAndQuery for more information.
func fetchAndExec[ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Exec func(ctx context.Context, arg ArgumentType) error](
	// Arguments
	logger slog.Logger,
	authorizer rbac.Authorizer,
	action rbac.Action,
	fetchFunc Fetch,
	execFunc Exec) Exec {
	f := fetchAndQuery(logger, authorizer, action, fetchFunc, func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		return empty, execFunc(ctx, arg)
	})
	return func(ctx context.Context, arg ArgumentType) error {
		_, err := f(ctx, arg)
		return err
	}
}

// fetchAndQuery is a generic function that wraps a database fetch and query.
// The fetch is used to know which rbac object the action should be asserted on
// **before** the query runs. The returns from the fetch are only used to
// assert rbac. The final return of this function comes from the Query function.
func fetchAndQuery[ObjectType rbac.Objecter, ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Query func(ctx context.Context, arg ArgumentType) (ObjectType, error)](
	// Arguments
	logger slog.Logger,
	authorizer rbac.Authorizer,
	action rbac.Action,
	fetchFunc Fetch,
	queryFunc Query) Query {
	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := ActorFromContext(ctx)
		if !ok {
			return empty, NoActorError
		}

		// Fetch the database object
		object, err := fetchFunc(ctx, arg)
		if err != nil {
			return empty, xerrors.Errorf("fetch object: %w", err)
		}

		// Authorize the action
		err = authorizer.Authorize(ctx, act, action, object.RBACObject())
		if err != nil {
			return empty, LogNotAuthorizedError(ctx, logger, err)
		}

		return queryFunc(ctx, arg)
	}
}

// fetchWithPostFilter is like fetch, but works with lists of objects.
// SQL filters are much more optimal.
func fetchWithPostFilter[ArgumentType any, ObjectType rbac.Objecter,
	DatabaseFunc func(ctx context.Context, arg ArgumentType) ([]ObjectType, error)](
	// Arguments
	authorizer rbac.Authorizer,
	f DatabaseFunc) DatabaseFunc {
	return func(ctx context.Context, arg ArgumentType) (empty []ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := ActorFromContext(ctx)
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

// prepareSQLFilter is a helper function that prepares a SQL filter using the
// given authorization context.
func prepareSQLFilter(ctx context.Context, authorizer rbac.Authorizer, action rbac.Action, resourceType string) (rbac.PreparedAuthorized, error) {
	act, ok := ActorFromContext(ctx)
	if !ok {
		return nil, xerrors.Errorf("no authorization actor in context")
	}

	return authorizer.Prepare(ctx, act, action, resourceType)
}
