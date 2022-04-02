// Package database connects to external services for stateful storage.
//
// Query functions are generated using sqlc.
//
// To modify the database schema:
// 1. Add a new migration using "create_migration.sh" in database/migrations/
// 2. Run "make coderd/database/generate" in the root to generate models.
// 3. Add/Edit queries in "query.sql" and run "make coderd/database/generate" to create Go code.
package database

import (
	"context"
	"errors"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"golang.org/x/xerrors"
)

var (
	ErrNoRows = pgx.ErrNoRows
)

// Store contains all queryable database functions.
// It extends the generated interface to add transaction support.
type Store interface {
	querier

	InTx(context.Context, func(Store) error) error
}

// DBTX represents a database connection or transaction.
type DBTX interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}

// New creates a new database store using a SQL database connection.
func New(pool *pgxpool.Pool) Store {

	return &sqlQuerier{
		db:   pool,
		pool: pool,
	}
}

type sqlQuerier struct {
	pool *pgxpool.Pool
	db   DBTX
}

// InTx performs database operations inside a transaction.
func (q *sqlQuerier) InTx(ctx context.Context, function func(Store) error) error {
	if q.pool == nil {
		return nil
	}

	transaction, err := q.pool.Begin(ctx)
	if err != nil {
		return xerrors.Errorf("begin transaction: %w", err)
	}

	defer func() {
		rerr := transaction.Rollback(ctx)
		if rerr == nil || errors.Is(rerr, pgx.ErrTxClosed) {
			// no need to do anything, tx committed successfully
			return
		}

		// couldn't roll back for some reason, extend returned error
		err = xerrors.Errorf("defer (%s): %w", rerr.Error(), err)
	}()

	err = function(&sqlQuerier{db: transaction})
	if err != nil {
		return xerrors.Errorf("execute transaction: %w", err)
	}

	err = transaction.Commit(ctx)
	if err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}

	return nil
}
