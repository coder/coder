package authzquery

import (
	"context"
	"database/sql"
	"time"

	"golang.org/x/xerrors"

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
	database   database.Store
	authorizer rbac.Authorizer
}

func NewAuthzQuerier(db database.Store, authorizer rbac.Authorizer) *AuthzQuerier {
	return &AuthzQuerier{
		database:   db,
		authorizer: authorizer,
	}
}

func (q *AuthzQuerier) Ping(ctx context.Context) (time.Duration, error) {
	return q.database.Ping(ctx)
}

// InTx runs the given function in a transaction.
// TODO: The method signature needs to be switched to use 'AuthzStore'. Until that
// interface is defined as a subset of database.Store, it would not compile.
// So use this method signature for now.
// func (q *AuthzQuerier) InTx(function func(querier AuthzStore) error, txOpts *sql.TxOptions) error {
func (q *AuthzQuerier) InTx(function func(querier database.Store) error, txOpts *sql.TxOptions) error {
	// TODO: @emyrk verify this works.
	return q.database.InTx(func(tx database.Store) error {
		// Wrap the transaction store in an AuthzQuerier.
		wrapped := NewAuthzQuerier(tx, q.authorizer)
		return function(wrapped)
	}, txOpts)
}

// authorizeContext is a helper function to authorize an action on an object.
func (q *AuthzQuerier) authorizeContext(ctx context.Context, action rbac.Action, object rbac.Objecter) error {
	act, ok := actorFromContext(ctx)
	if !ok {
		return xerrors.Errorf("no authorization actor in context")
	}

	err := q.authorizer.ByRoleName(ctx, act.ID.String(), rbac.RoleNames(act.Roles), act.Scope, act.Groups, action, object.RBACObject())
	if err != nil {
		return xerrors.Errorf("unauthorized: %w", err)
	}
	return nil
}

type fetchObjFunc func() (rbac.Objecter, error)

// authorizeContextF is a helper function to authorize an action on an object.
// objectFunc is a function that returns the object on which to authorize.
func (q *AuthzQuerier) authorizeContextF(ctx context.Context, action rbac.Action, fetchObj fetchObjFunc) error {
	obj, err := fetchObj()
	if err != nil {
		return xerrors.Errorf("fetch rbac object: %w", err)
	}
	return q.authorizeContext(ctx, action, obj.RBACObject())
}
