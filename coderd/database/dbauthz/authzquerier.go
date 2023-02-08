package dbauthz

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

var _ database.Store = (*AuthzQuerier)(nil)

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

// AuthzQuerier is a wrapper around the database store that performs authorization
// checks before returning data. All AuthzQuerier methods expect an authorization
// subject present in the context. If no subject is present, most methods will
// fail.
//
// Use WithAuthorizeContext to set the authorization subject in the context for
// the common user case.
type AuthzQuerier struct {
	db   database.Store
	auth rbac.Authorizer
	log  slog.Logger
}

func New(db database.Store, authorizer rbac.Authorizer, logger slog.Logger) *AuthzQuerier {
	return &AuthzQuerier{
		db:   db,
		auth: authorizer,
		log:  logger,
	}
}

func (q *AuthzQuerier) Ping(ctx context.Context) (time.Duration, error) {
	return q.db.Ping(ctx)
}

// InTx runs the given function in a transaction.
func (q *AuthzQuerier) InTx(function func(querier database.Store) error, txOpts *sql.TxOptions) error {
	return q.db.InTx(func(tx database.Store) error {
		// Wrap the transaction store in an AuthzQuerier.
		wrapped := New(tx, q.auth, q.log)
		return function(wrapped)
	}, txOpts)
}

// authorizeContext is a helper function to authorize an action on an object.
func (q *AuthzQuerier) authorizeContext(ctx context.Context, action rbac.Action, object rbac.Objecter) error {
	act, ok := ActorFromContext(ctx)
	if !ok {
		return NoActorError
	}

	err := q.auth.Authorize(ctx, act, action, object.RBACObject())
	if err != nil {
		return LogNotAuthorizedError(ctx, q.log, err)
	}
	return nil
}
