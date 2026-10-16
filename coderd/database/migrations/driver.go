package migrations

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/lib/pq"
	"golang.org/x/xerrors"
)

const (
	lockID              = int64(1037453835920848937)
	migrationsTableName = "schema_migrations"

	// NoTransactionMarker opts a migration file out of running inside a
	// transaction. It must appear as a leading comment, before any SQL, and
	// the file must contain exactly one statement. Postgres runs a
	// multi-statement simple query in an implicit transaction block, which
	// would reject CREATE INDEX CONCURRENTLY just like an explicit one.
	NoTransactionMarker = "-- coder:no-transaction"
)

// pgDriver is a Postgres migration driver that serializes concurrent runs
// with a session-level advisory lock and commits each migration in its own
// transaction together with its schema_migrations row, so the recorded version
// can never diverge from the applied schema.
//
// A migration that starts with NoTransactionMarker runs outside any
// transaction, which is what CREATE INDEX CONCURRENTLY needs. Only those
// migrations can leave schema_migrations dirty: the row is marked dirty before
// the statement runs and clean after it succeeds, so a failure in between is
// visible to the operator and blocks further migrations until resolved.
type pgDriver struct {
	ctx context.Context
	db  *sql.DB
	// lockTimeout bounds how long any migration statement waits for a
	// relation lock. Zero disables the bound.
	lockTimeout time.Duration

	// conn holds the advisory lock for the duration of a batch. Every
	// migration in the batch runs on it.
	conn *sql.Conn
	// tx is the transaction for the migration currently being applied, or nil
	// between migrations and while a no-transaction migration runs.
	tx *sql.Tx
}

func (*pgDriver) Open(string) (database.Driver, error) {
	panic("not implemented")
}

func (*pgDriver) Close() error {
	return nil
}

// Lock takes the batch advisory lock on a dedicated connection. Postgres
// lock_timeout also applies to advisory locks, so it is set only after the
// advisory lock is held. Otherwise a replica queued behind a concurrent
// migration would time out instead of waiting for up to migrate.LockTimeout.
func (d *pgDriver) Lock() error {
	conn, err := d.db.Conn(d.ctx)
	if err != nil {
		return xerrors.Errorf("acquire connection: %w", err)
	}

	_, err = conn.ExecContext(d.ctx, `SELECT pg_advisory_lock($1)`, lockID)
	if err != nil {
		discardConn(conn)
		return xerrors.Errorf("acquire advisory lock: %w", err)
	}

	// SET does not accept bind parameters, so the millisecond value is
	// formatted into the query. The setting is session scoped and undone in
	// Unlock before the connection returns to the pool.
	if d.lockTimeout > 0 {
		ms := max(d.lockTimeout.Milliseconds(), 1)
		_, err = conn.ExecContext(d.ctx, fmt.Sprintf("SET lock_timeout = %d", ms))
		if err != nil {
			discardConn(conn)
			return xerrors.Errorf("set lock_timeout: %w", err)
		}
	}

	d.conn = conn
	return nil
}

// Unlock rolls back any migration still in flight, releases the advisory lock,
// and returns the connection to the pool. If the session cannot be restored
// to its pooled state the connection is discarded instead so the lock and
// lock_timeout cannot leak into application queries.
func (d *pgDriver) Unlock() error {
	conn := d.conn
	if conn == nil {
		return nil
	}
	d.conn = nil

	var errs []error
	if d.tx != nil {
		if err := d.tx.Rollback(); err != nil && !xerrors.Is(err, sql.ErrTxDone) {
			errs = append(errs, xerrors.Errorf("rollback in-flight migration: %w", err))
		}
		d.tx = nil
	}
	if len(errs) == 0 {
		if _, err := conn.ExecContext(d.ctx, "RESET lock_timeout"); err != nil {
			errs = append(errs, xerrors.Errorf("reset lock_timeout: %w", err))
		}
	}
	if len(errs) == 0 {
		if _, err := conn.ExecContext(d.ctx, `SELECT pg_advisory_unlock($1)`, lockID); err != nil {
			errs = append(errs, xerrors.Errorf("release advisory lock: %w", err))
		}
	}
	if len(errs) > 0 {
		discardConn(conn)
		return xerrors.Errorf("unlock: %w", errs[0])
	}
	if err := conn.Close(); err != nil {
		return xerrors.Errorf("release connection: %w", err)
	}
	return nil
}

// discardConn closes the underlying database connection instead of returning
// it to the pool. Session state such as the advisory lock and lock_timeout
// dies with it.
func discardConn(conn *sql.Conn) {
	_ = conn.Raw(func(any) error {
		return driver.ErrBadConn
	})
	_ = conn.Close()
}

// Run executes one migration. golang-migrate calls SetVersion(v, true) first,
// which opened d.tx, and SetVersion(v, false) afterwards, which commits it.
func (d *pgDriver) Run(migration io.Reader) error {
	migr, err := io.ReadAll(migration)
	if err != nil {
		return xerrors.Errorf("read migration: %w", err)
	}
	query := string(migr)
	if strings.TrimSpace(query) == "" {
		return nil
	}

	if isNoTransaction(migr) {
		// Persist the dirty marker on its own so a failure below is recorded,
		// then run the statement in autocommit mode.
		if err := d.commitTx(); err != nil {
			return err
		}
		_, err = d.conn.ExecContext(d.ctx, query)
	} else {
		if d.tx == nil {
			return xerrors.New("migration run without an open transaction")
		}
		_, err = d.tx.ExecContext(d.ctx, query)
	}
	if err != nil {
		return wrapMigrationError(err, migr)
	}
	return nil
}

func wrapMigrationError(err error, statement []byte) error {
	var pgErr *pq.Error
	if xerrors.As(err, &pgErr) {
		message := fmt.Sprintf("migration failed: %s", pgErr.Message)
		if pgErr.Detail != "" {
			message += ", " + pgErr.Detail
		}
		return database.Error{OrigErr: err, Err: message, Query: statement}
	}
	return database.Error{OrigErr: err, Err: "migration failed", Query: statement}
}

// isNoTransaction reports whether the migration opts out of running in a
// transaction. The marker must appear in the leading comment block, before
// any SQL.
func isNoTransaction(migration []byte) bool {
	scanner := bufio.NewScanner(bytes.NewReader(migration))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "":
			continue
		case line == NoTransactionMarker:
			return true
		case strings.HasPrefix(line, "--"):
			continue
		default:
			return false
		}
	}
	return false
}

// SetVersion records the migration version. Marking a version dirty opens the
// transaction for that migration; marking it clean commits it. When no
// transaction is open, for example after a no-transaction migration or for a
// migration with an empty body, the row is written in autocommit mode.
//
//nolint:revive
func (d *pgDriver) SetVersion(version int, dirty bool) error {
	if dirty && d.tx == nil {
		tx, err := d.conn.BeginTx(d.ctx, nil)
		if err != nil {
			return xerrors.Errorf("begin migration transaction: %w", err)
		}
		d.tx = tx
	}

	var exec interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	} = d.conn
	if d.tx != nil {
		exec = d.tx
	}

	query := `TRUNCATE ` + migrationsTableName
	if _, err := exec.ExecContext(d.ctx, query); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	if version >= 0 {
		query = `INSERT INTO ` + migrationsTableName + ` (version, dirty) VALUES ($1, $2)`
		if _, err := exec.ExecContext(d.ctx, query, version, dirty); err != nil {
			return &database.Error{OrigErr: err, Query: []byte(query)}
		}
	}

	if !dirty {
		return d.commitTx()
	}
	return nil
}

// commitTx commits the current migration transaction, if any.
func (d *pgDriver) commitTx() error {
	if d.tx == nil {
		return nil
	}
	tx := d.tx
	d.tx = nil
	if err := tx.Commit(); err != nil {
		return xerrors.Errorf("commit migration transaction: %w", err)
	}
	return nil
}

func (d *pgDriver) Version() (version int, dirty bool, err error) {
	// Read through the locked connection when we hold it so the result is
	// consistent with the batch. Stepper and EnsureClean call this without
	// the lock and read from the pool instead.
	var q interface {
		QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	} = d.db
	if d.tx != nil {
		q = d.tx
	} else if d.conn != nil {
		q = d.conn
	}

	query := `SELECT version, dirty FROM ` + migrationsTableName + ` LIMIT 1`
	err = q.QueryRowContext(d.ctx, query).Scan(&version, &dirty)
	switch {
	case err == sql.ErrNoRows:
		return database.NilVersion, false, nil

	case err != nil:
		var pgErr *pq.Error
		if xerrors.As(err, &pgErr) {
			if pgErr.Code.Name() == "undefined_table" {
				return database.NilVersion, false, nil
			}
		}
		return 0, false, &database.Error{OrigErr: err, Query: []byte(query)}

	default:
		return version, dirty, nil
	}
}

func (*pgDriver) Drop() error {
	panic("not implemented")
}

func (d *pgDriver) ensureVersionTable() error {
	err := d.Lock()
	if err != nil {
		return xerrors.Errorf("acquire migration lock: %w", err)
	}

	const query = `CREATE TABLE IF NOT EXISTS ` + migrationsTableName + ` (version bigint not null primary key, dirty boolean not null)`
	if _, err := d.conn.ExecContext(d.ctx, query); err != nil {
		_ = d.Unlock()
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}

	err = d.Unlock()
	if err != nil {
		return xerrors.Errorf("release migration lock: %w", err)
	}

	return nil
}
