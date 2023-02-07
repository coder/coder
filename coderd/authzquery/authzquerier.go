package authzquery

import (
	"context"
	"database/sql"
	"time"

	"cdr.dev/slog"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

var _ database.Store = (*AuthzQuerier)(nil)

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
// TODO: The method signature needs to be switched to use 'AuthzStore'. Until that
// interface is defined as a subset of database.Store, it would not compile.
// So use this method signature for now.
// func (q *AuthzQuerier) InTx(function func(querier AuthzStore) error, txOpts *sql.TxOptions) error {
func (q *AuthzQuerier) InTx(function func(querier database.Store) error, txOpts *sql.TxOptions) error {
	// TODO: @emyrk verify this works.
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
