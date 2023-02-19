package dbauthz

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/open-policy-agent/opa/topdown"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

var _ database.Store = (*querier)(nil)

// NoActorError wraps ErrNoRows for the api to return a 404. This is the correct
// response when the user is not authorized.
var NoActorError = xerrors.Errorf("no authorization actor in context: %w", sql.ErrNoRows)

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

func logNotAuthorizedError(ctx context.Context, logger slog.Logger, err error) error {
	// Only log the errors if it is an UnauthorizedError error.
	internalError := new(rbac.UnauthorizedError)
	if err != nil && xerrors.As(err, &internalError) {
		e := new(topdown.Error)
		if xerrors.As(err, &e) || e.Code == topdown.CancelErr {
			// For some reason rego changes a canceled context to a topdown.CancelErr. We
			// expect to check for canceled context errors if the user cancels the request,
			// so we should change the error to a context.Canceled error.
			//
			// NotAuthorizedError is == to sql.ErrNoRows, which is not correct
			// if it's actually a canceled context.
			internalError.SetInternal(context.Canceled)
			return internalError
		}
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

// querier is a wrapper around the database store that performs authorization
// checks before returning data. All querier methods expect an authorization
// subject present in the context. If no subject is present, most methods will
// fail.
//
// Use WithAuthorizeContext to set the authorization subject in the context for
// the common user case.
type querier struct {
	db   database.Store
	auth rbac.Authorizer
	log  slog.Logger
}

func New(db database.Store, authorizer rbac.Authorizer, logger slog.Logger) database.Store {
	// If the underlying db store is already a querier, return it.
	// Do not double wrap.
	if _, ok := db.(*querier); ok {
		return db
	}
	return &querier{
		db:   db,
		auth: authorizer,
		log:  logger,
	}
}

// authorizeContext is a helper function to authorize an action on an object.
func (q *querier) authorizeContext(ctx context.Context, action rbac.Action, object rbac.Objecter) error {
	act, ok := ActorFromContext(ctx)
	if !ok {
		return NoActorError
	}

	err := q.auth.Authorize(ctx, act, action, object.RBACObject())
	if err != nil {
		return logNotAuthorizedError(ctx, q.log, err)
	}
	return nil
}

type authContextKey struct{}

// ActorFromContext returns the authorization subject from the context.
// All authentication flows should set the authorization subject in the context.
// If no actor is present, the function returns false.
func ActorFromContext(ctx context.Context) (rbac.Subject, bool) {
	a, ok := ctx.Value(authContextKey{}).(rbac.Subject)
	return a, ok
}

// AsProvisionerd returns a context with an actor that has permissions required
// for provisionerd to function.
func AsProvisionerd(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, rbac.Subject{
		ID: uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Name:        "provisionerd",
				DisplayName: "Provisioner Daemon",
				Site: rbac.Permissions(map[string][]rbac.Action{
					rbac.ResourceFile.Type:      {rbac.ActionRead},
					rbac.ResourceTemplate.Type:  {rbac.ActionRead, rbac.ActionUpdate},
					rbac.ResourceUser.Type:      {rbac.ActionRead},
					rbac.ResourceWorkspace.Type: {rbac.ActionRead, rbac.ActionUpdate, rbac.ActionDelete},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	},
	)
}

// AsAutostart returns a context with an actor that has permissions required
// for autostart to function.
func AsAutostart(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, rbac.Subject{
		ID: uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Name:        "autostart",
				DisplayName: "Autostart Daemon",
				Site: rbac.Permissions(map[string][]rbac.Action{
					rbac.ResourceTemplate.Type:  {rbac.ActionRead, rbac.ActionUpdate},
					rbac.ResourceWorkspace.Type: {rbac.ActionRead, rbac.ActionUpdate},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	},
	)
}

// AsSystemRestricted returns a context with an actor that has permissions
// required for various system operations (login, logout, metrics cache).
func AsSystemRestricted(ctx context.Context) context.Context {
	return context.WithValue(ctx, authContextKey{}, rbac.Subject{
		ID: uuid.Nil.String(),
		Roles: rbac.Roles([]rbac.Role{
			{
				Name:        "system",
				DisplayName: "Coder",
				Site: rbac.Permissions(map[string][]rbac.Action{
					rbac.ResourceWildcard.Type:           {rbac.ActionRead},
					rbac.ResourceAPIKey.Type:             {rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
					rbac.ResourceGroup.Type:              {rbac.ActionCreate, rbac.ActionUpdate},
					rbac.ResourceRoleAssignment.Type:     {rbac.ActionCreate},
					rbac.ResourceOrganization.Type:       {rbac.ActionCreate},
					rbac.ResourceOrganizationMember.Type: {rbac.ActionCreate},
					rbac.ResourceOrgRoleAssignment.Type:  {rbac.ActionCreate},
					rbac.ResourceUser.Type:               {rbac.ActionCreate, rbac.ActionUpdate, rbac.ActionDelete},
					rbac.ResourceUserData.Type:           {rbac.ActionCreate, rbac.ActionUpdate},
					rbac.ResourceWorkspace.Type:          {rbac.ActionUpdate},
				}),
				Org:  map[string][]rbac.Permission{},
				User: []rbac.Permission{},
			},
		}),
		Scope: rbac.ScopeAll,
	},
	)
}

var AsRemoveActor = rbac.Subject{
	ID: "remove-actor",
}

// As returns a context with the given actor stored in the context.
// This is used for cases where the actor touching the database is not the
// actor stored in the context.
// When you use this function, be sure to add a //nolint comment
// explaining why it is necessary.
func As(ctx context.Context, actor rbac.Subject) context.Context {
	if actor.Equal(AsRemoveActor) {
		// AsRemoveActor is a special case that is used to indicate that the actor
		// should be removed from the context.
		return context.WithValue(ctx, authContextKey{}, nil)
	}
	return context.WithValue(ctx, authContextKey{}, actor)
}

//
// Generic functions used to implement the database.Store methods.
//

// insert runs an rbac.ActionCreate on the rbac object argument before
// running the insertFunc. The insertFunc is expected to return the object that
// was inserted.
func insert[
	ObjectType any,
	ArgumentType any,
	Insert func(ctx context.Context, arg ArgumentType) (ObjectType, error),
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	object rbac.Objecter,
	insertFunc Insert,
) Insert {
	return func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := ActorFromContext(ctx)
		if !ok {
			return empty, NoActorError
		}

		// Authorize the action
		err = authorizer.Authorize(ctx, act, rbac.ActionCreate, object.RBACObject())
		if err != nil {
			return empty, logNotAuthorizedError(ctx, logger, err)
		}

		// Insert the database object
		return insertFunc(ctx, arg)
	}
}

func deleteQ[
	ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Delete func(ctx context.Context, arg ArgumentType) error,
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	deleteFunc Delete,
) Delete {
	return fetchAndExec(logger, authorizer,
		rbac.ActionDelete, fetchFunc, deleteFunc)
}

func updateWithReturn[
	ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	UpdateQuery func(ctx context.Context, arg ArgumentType) (ObjectType, error),
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	updateQuery UpdateQuery,
) UpdateQuery {
	return fetchAndQuery(logger, authorizer, rbac.ActionUpdate, fetchFunc, updateQuery)
}

func update[
	ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Exec func(ctx context.Context, arg ArgumentType) error,
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	fetchFunc Fetch,
	updateExec Exec,
) Exec {
	return fetchAndExec(logger, authorizer, rbac.ActionUpdate, fetchFunc, updateExec)
}

// fetch is a generic function that wraps a database
// query function (returns an object and an error) with authorization. The
// returned function has the same arguments as the database function.
//
// The database query function will **ALWAYS** hit the database, even if the
// user cannot read the resource. This is because the resource details are
// required to run a proper authorization check.
func fetch[
	ArgumentType any,
	ObjectType rbac.Objecter,
	DatabaseFunc func(ctx context.Context, arg ArgumentType) (ObjectType, error),
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	f DatabaseFunc,
) DatabaseFunc {
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
			return empty, logNotAuthorizedError(ctx, logger, err)
		}

		return object, nil
	}
}

// fetchAndExec uses fetchAndQuery but only returns the error. The naming comes
// from SQL 'exec' functions which only return an error.
// See fetchAndQuery for more information.
func fetchAndExec[
	ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Exec func(ctx context.Context, arg ArgumentType) error,
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	action rbac.Action,
	fetchFunc Fetch,
	execFunc Exec,
) Exec {
	f := fetchAndQuery(logger, authorizer, action, fetchFunc, func(ctx context.Context, arg ArgumentType) (empty ObjectType, err error) {
		return empty, execFunc(ctx, arg)
	})
	return func(ctx context.Context, arg ArgumentType) error {
		_, err := f(ctx, arg)
		return err
	}
}

// fetchAndQuery is a generic function that wraps a database fetch and query.
// A query has potential side effects in the database (update, delete, etc).
// The fetch is used to know which rbac object the action should be asserted on
// **before** the query runs. The returns from the fetch are only used to
// assert rbac. The final return of this function comes from the Query function.
func fetchAndQuery[
	ObjectType rbac.Objecter,
	ArgumentType any,
	Fetch func(ctx context.Context, arg ArgumentType) (ObjectType, error),
	Query func(ctx context.Context, arg ArgumentType) (ObjectType, error),
](
	logger slog.Logger,
	authorizer rbac.Authorizer,
	action rbac.Action,
	fetchFunc Fetch,
	queryFunc Query,
) Query {
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
			return empty, logNotAuthorizedError(ctx, logger, err)
		}

		return queryFunc(ctx, arg)
	}
}

// fetchWithPostFilter is like fetch, but works with lists of objects.
// SQL filters are much more optimal.
func fetchWithPostFilter[
	ArgumentType any,
	ObjectType rbac.Objecter,
	DatabaseFunc func(ctx context.Context, arg ArgumentType) ([]ObjectType, error),
](
	authorizer rbac.Authorizer,
	f DatabaseFunc,
) DatabaseFunc {
	return func(ctx context.Context, arg ArgumentType) (empty []ObjectType, err error) {
		// Fetch the rbac subject
		act, ok := ActorFromContext(ctx)
		if !ok {
			return empty, NoActorError
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
		return nil, NoActorError
	}

	return authorizer.Prepare(ctx, act, action, resourceType)
}
