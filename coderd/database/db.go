// Package database connects to external services for stateful storage.
//
// Query functions are generated using sqlc.
//
// To modify the database schema:
// 1. Add a new migration using "create_migration.sh" in database/migrations/ and run "make gen" to generate models.
// 2. Add/Edit queries in "query.sql" and run "make gen" to create Go code.
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
	// wrapper allows us to detect if the interface has been wrapped.
	wrapper

	Ping(ctx context.Context) (time.Duration, error)
	PGLocks(ctx context.Context) (PGLocks, error)
	InTx(func(Store) error, *TxOptions) error
}

type wrapper interface {
	// Wrappers returns a list of wrappers that have been applied to the store.
	// This is used to detect if the store has already wrapped, and avoid
	// double-wrapping.
	Wrappers() []string
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

func WithSerialRetryCount(count int) func(*sqlQuerier) {
	return func(q *sqlQuerier) {
		q.serialRetryCount = count
	}
}

// New creates a new database store using a SQL database connection.
func New(sdb *sql.DB, opts ...func(*sqlQuerier)) Store {
	dbx := sqlx.NewDb(sdb, "postgres")
	q := &sqlQuerier{
		db:  dbx,
		sdb: dbx,
		// This is an arbitrary number.
		serialRetryCount: 3,
	}

	for _, opt := range opts {
		opt(q)
	}
	return q
}

// TxOptions configures a transaction: its isolation level, read-only mode,
// and serialization retry policy. It also carries Coder-specific metadata
// for metrics: TxIdentifier (set by the caller) and executionCount (set by
// InTx). Ideally that metadata would go in a context, but no context is used for transactions,
// so it is attached to the options passed in instead.
// The metadata should not be returned in the method signature, because it
// is only used for metric tracking. It should never be used by business logic.
type TxOptions struct {
	// Isolation is the transaction isolation level.
	// If zero, the driver or database's default level is used.
	Isolation sql.IsolationLevel
	ReadOnly  bool

	// SerialMaxAttempts overrides the store's default number of attempts for
	// serializable transactions that fail with a serialization error. Zero
	// uses the default. A non-zero value takes precedence over the store's
	// count, including WithSerialRetryCount in tests.
	SerialMaxAttempts int
	// BeforeSerialRetry, if set, is called between serializable attempts,
	// after the failed transaction has been rolled back, with the number of
	// attempts made so far. A non-nil error stops retrying and is returned.
	// No connection or transaction is held while it runs, but time spent in
	// it (for example, backoff waits) is included in the transaction
	// duration metric.
	//
	// Both fields only apply to the outermost serializable InTx call. They are
	// ignored inside an existing transaction, where the outer call owns retries.
	BeforeSerialRetry func(attempt int) error

	// -- Coder specific metadata --
	// TxIdentifier is a unique identifier for the transaction to be used
	// in metrics. Can be any string.
	TxIdentifier string

	// Set by InTx
	executionCount int
}

// IncrementExecutionCount is a helper function for external packages
// to increment the unexported count.
func IncrementExecutionCount(opts *TxOptions) {
	opts.executionCount++
}

func (o TxOptions) ExecutionCount() int {
	return o.executionCount
}

func (o *TxOptions) WithID(id string) *TxOptions {
	o.TxIdentifier = id
	return o
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

	// serialRetryCount is the number of times to retry a transaction
	// if it fails with a serialization error.
	serialRetryCount int
}

func (*sqlQuerier) Wrappers() []string {
	return []string{}
}

// Ping returns the time it takes to ping the database.
func (q *sqlQuerier) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	err := q.sdb.PingContext(ctx)
	return time.Since(start), err
}

func DefaultTXOptions() *TxOptions {
	return &TxOptions{
		Isolation: sql.LevelDefault,
		ReadOnly:  false,
	}
}

func (q *sqlQuerier) InTx(function func(Store) error, txOpts *TxOptions) error {
	_, inTx := q.db.(*sqlx.Tx)

	if txOpts == nil {
		// create a default txOpts if left to nil
		txOpts = DefaultTXOptions()
	}

	sqlOpts := &sql.TxOptions{
		Isolation: txOpts.Isolation,
		ReadOnly:  txOpts.ReadOnly,
	}

	// If we are not already in a transaction, and we are running in serializable
	// mode, we need to run the transaction in a retry loop. The caller should be
	// prepared to allow retries if using serializable mode.
	// If we are in a transaction already, the parent InTx call will handle the retry.
	// We do not want to duplicate those retries.
	if !inTx && sqlOpts.Isolation == sql.LevelSerializable {
		maxAttempts := q.serialRetryCount
		if txOpts.SerialMaxAttempts > 0 {
			maxAttempts = txOpts.SerialMaxAttempts
		}
		var err error
		attempts := 0
		for attempts = 0; attempts < maxAttempts; attempts++ {
			if attempts > 0 && txOpts.BeforeSerialRetry != nil {
				if hookErr := txOpts.BeforeSerialRetry(attempts); hookErr != nil {
					return xerrors.Errorf("stop transaction retry after attempt %d (last error: %v): %w", attempts, err, hookErr)
				}
			}
			txOpts.executionCount++
			err = q.runTx(function, sqlOpts)
			if err == nil {
				// Transaction succeeded.
				return nil
			}
			if !IsSerializedError(err) {
				// We should only retry if the error is a serialization error.
				return err
			}
		}
		// Transaction kept failing in serializable mode.
		return xerrors.Errorf("transaction failed after %d attempts: %w", attempts, err)
	}

	txOpts.executionCount++
	return q.runTx(function, sqlOpts)
}

// InTx performs database operations inside a transaction.
func (q *sqlQuerier) runTx(function func(Store) error, txOpts *sql.TxOptions) (err error) {
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

func safeString(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
