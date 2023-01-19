package authzquery

import (
	"context"
	"database/sql"
	"time"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/rbac"
)

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

func (q *AuthzQuerier) InTx(function func(database.Store) error, txOpts *sql.TxOptions) error {
	// TODO: @emyrk verify this works.
	return q.database.InTx(func(tx database.Store) error {
		// Wrap the transaction store in an AuthzQuerier.
		wrapped := NewAuthzQuerier(tx, q.authorizer)
		return function(wrapped)
	}, txOpts)
}
