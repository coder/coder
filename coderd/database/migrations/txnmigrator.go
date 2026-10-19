package migrations

import (
	"context"
	"database/sql"
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
)

// pgTxnDriver is a Postgres migration driver that runs all migrations in a
// single transaction. This is done to prevent users from being locked out of
// their deployment if a migration fails, since the schema will simply revert
// back to the previous version.
type pgTxnDriver struct {
	ctx context.Context
	db  *sql.DB
	tx  *sql.Tx

	reporter *reporter
}

func (*pgTxnDriver) Open(string) (database.Driver, error) {
	panic("not implemented")
}

func (*pgTxnDriver) Close() error {
	return nil
}

func (d *pgTxnDriver) Lock() error {
	var err error

	d.tx, err = d.db.BeginTx(d.ctx, nil)
	if err != nil {
		return err
	}
	if d.reporter != nil {
		return d.lockWithReporting()
	}
	const q = `
SELECT pg_advisory_xact_lock($1)
`

	_, err = d.tx.ExecContext(d.ctx, q, lockID)
	if err != nil {
		return xerrors.Errorf("exec select: %w", err)
	}
	return nil
}

// lockWithReporting acquires the same transaction-scoped advisory lock as
// Lock, and reports when it has to wait for another instance.
// Logs only when it has to wait, a free lock is taken silently.
func (d *pgTxnDriver) lockWithReporting() error {
	if d.reporter.appName != "" {
		// Transaction-local, so the pooled connection is unaffected afterwards.
		if _, err := d.tx.ExecContext(d.ctx, `SELECT set_config('application_name', $1, true)`, d.reporter.appName); err != nil {
			return xerrors.Errorf("set application name: %w", err)
		}
	}
	var pid int
	if err := d.tx.QueryRowContext(d.ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		return xerrors.Errorf("get backend pid: %w", err)
	}
	d.reporter.setBackend(pid)

	var acquired bool
	if err := d.tx.QueryRowContext(d.ctx, `SELECT pg_try_advisory_xact_lock($1)`, lockID).Scan(&acquired); err != nil {
		return xerrors.Errorf("try advisory lock: %w", err)
	}
	if acquired {
		return nil
	}

	start := time.Now()
	stop := d.reporter.lockWaitStarted(pid)
	_, err := d.tx.ExecContext(d.ctx, `SELECT pg_advisory_xact_lock($1)`, lockID)
	stop()
	if err != nil {
		return xerrors.Errorf("exec select: %w", err)
	}

	d.reporter.lockAcquired(time.Since(start))

	return nil
}

func (d *pgTxnDriver) Unlock() error {
	d.reporter.stopRunningHeartbeat()
	err := d.tx.Commit()
	d.tx = nil
	if err != nil {
		return xerrors.Errorf("commit tx on unlock: %w", err)
	}
	return nil
}

func (d *pgTxnDriver) Run(migration io.Reader) error {
	migr, err := io.ReadAll(migration)
	if err != nil {
		return xerrors.Errorf("read migration: %w", err)
	}
	err = d.runStatement(migr)
	if err != nil {
		return xerrors.Errorf("run statement: %w", err)
	}
	return nil
}

func (d *pgTxnDriver) runStatement(statement []byte) error {
	ctx := context.Background()
	query := string(statement)
	if strings.TrimSpace(query) == "" {
		return nil
	}
	if _, err := d.tx.ExecContext(ctx, query); err != nil {
		d.reporter.statementFailed(err)
		var pgErr *pq.Error
		if xerrors.As(err, &pgErr) {
			var line uint
			message := fmt.Sprintf("migration failed: %s", pgErr.Message)
			if pgErr.Detail != "" {
				message += ", " + pgErr.Detail
			}
			return database.Error{OrigErr: err, Err: message, Query: statement, Line: line}
		}
		return database.Error{OrigErr: err, Err: "migration failed", Query: statement}
	}
	return nil
}

//nolint:revive
func (d *pgTxnDriver) SetVersion(version int, dirty bool) error {
	query := `TRUNCATE ` + migrationsTableName
	if _, err := d.tx.Exec(query); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}

	if version >= 0 {
		query = `INSERT INTO ` + migrationsTableName + ` (version, dirty) VALUES ($1, $2)`
		if _, err := d.tx.Exec(query, version, dirty); err != nil {
			return &database.Error{OrigErr: err, Query: []byte(query)}
		}
	}

	if dirty {
		d.reporter.migrationStarted(version)
	} else {
		d.reporter.migrationApplied(version)
	}
	return nil
}

func (d *pgTxnDriver) Version() (version int, dirty bool, err error) {
	// If the transaction is valid (we hold the exclusive lock), use the txn for
	// the query.
	var q interface {
		QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	} = d.tx
	// If we don't hold the lock just use the database. This only happens in the
	// `Stepper` function and is only used in tests.
	if d.tx == nil {
		q = d.db
	}

	query := `SELECT version, dirty FROM ` + migrationsTableName + ` LIMIT 1`
	err = q.QueryRowContext(context.Background(), query).Scan(&version, &dirty)
	switch {
	case err == sql.ErrNoRows:
		version, dirty = database.NilVersion, false

	case err != nil:
		var pgErr *pq.Error
		if !xerrors.As(err, &pgErr) || pgErr.Code.Name() != "undefined_table" {
			return 0, false, &database.Error{OrigErr: err, Query: []byte(query)}
		}
		version, dirty = database.NilVersion, false
	}

	// golang-migrate reads the version once per run, right after taking the
	// lock. Capture the transaction ID there so the outcome can later be read
	// from PostgreSQL with pg_xact_status.
	if d.tx != nil && d.reporter != nil {
		var xid string
		if err := d.tx.QueryRowContext(context.Background(), `SELECT pg_current_xact_id()::text`).Scan(&xid); err != nil {
			return 0, false, &database.Error{OrigErr: err, Query: []byte(`SELECT pg_current_xact_id()`)}
		}
		d.reporter.inventory(version, dirty, xid)
	}
	return version, dirty, nil
}

func (*pgTxnDriver) Drop() error {
	panic("not implemented")
}

func (d *pgTxnDriver) ensureVersionTable() error {
	err := d.Lock()
	if err != nil {
		return xerrors.Errorf("acquire migration lock: %w", err)
	}

	const query = `CREATE TABLE IF NOT EXISTS ` + migrationsTableName + ` (version bigint not null primary key, dirty boolean not null)`
	if _, err := d.tx.ExecContext(context.Background(), query); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}

	err = d.Unlock()
	if err != nil {
		return xerrors.Errorf("release migration lock: %w", err)
	}

	return nil
}
