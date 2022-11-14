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
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"golang.org/x/xerrors"
)

// Store contains all queryable database functions.
// It extends the generated interface to add transaction support.
type Store interface {
	querier
	// customQuerier contains custom queries that are not generated.
	customQuerier

	Ping(ctx context.Context) (time.Duration, error)
	InTx(func(Store) error, *sql.TxOptions) error
}

// DBTX represents a database connection or transaction.
type DBTX interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
}

// New creates a new database store using a SQL database connection.
func New(sdb *sql.DB) Store {
	dbx := sqlx.NewDb(sdb, "postgres")
	return &sqlQuerier{
		db:  dbx,
		sdb: dbx,
	}
}

// queries encompasses both are sqlc generated
// queries and our custom queries.
type querier interface {
	sqlcQuerier
	customQuerier
}

type sqlQuerier struct {
	sdb *sqlx.DB
	db  DBTX
}

// Ping returns the time it takes to ping the database.
func (q *sqlQuerier) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	err := q.sdb.PingContext(ctx)
	return time.Since(start), err
}

// InTx performs database operations inside a transaction.
func (q *sqlQuerier) InTx(function func(Store) error, txOpts *sql.TxOptions) error {
	if _, ok := q.db.(*sqlx.Tx); ok {
		// If the current inner "db" is already a transaction, we just reuse it.
		// We do not need to handle commit/rollback as the outer tx will handle
		// that.
		err := function(q)
		if err != nil {
			return xerrors.Errorf("execute transaction: %w", err)
		}
		return nil
	}

	transaction, err := q.sdb.BeginTxx(context.Background(), txOpts)
	if err != nil {
		return xerrors.Errorf("begin transaction: %w", err)
	}
	defer func() {
		rerr := transaction.Rollback()
		if rerr == nil || errors.Is(rerr, sql.ErrTxDone) {
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
	err = transaction.Commit()
	if err != nil {
		return xerrors.Errorf("commit transaction: %w", err)
	}
	return nil
}
