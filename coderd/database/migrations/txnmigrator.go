package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
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

	logger slog.Logger
	// logCtx is used only for logging so the caller's context fields are
	// attached without letting its cancellation abort the migration
	// transaction.
	logCtx context.Context
	// source resolves migration versions to their names for logging.
	source source.Driver
	// inFlight is the version of the migration currently executing, or -1.
	inFlight      int
	inFlightStart time.Time
	// applied lists the versions applied in the current transaction. They are
	// not durable until the transaction commits in Unlock.
	applied []int
	// commitErr is the result of the commit in the most recent Unlock.
	commitErr error
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
	d.inFlight = -1
	d.applied = nil
	d.commitErr = nil

	// Try the lock first so that waiting on another instance, which holds it
	// for the duration of its migrations, is visible in the logs.
	var acquired bool
	err = d.tx.QueryRowContext(d.ctx, `SELECT pg_try_advisory_xact_lock($1)`, lockID).Scan(&acquired)
	if err != nil {
		return xerrors.Errorf("try advisory lock: %w", err)
	}
	if acquired {
		return nil
	}

	d.logger.Info(d.logCtx, "waiting for database migration lock held by another instance")
	start := time.Now()
	_, err = d.tx.ExecContext(d.ctx, `SELECT pg_advisory_xact_lock($1)`, lockID)
	if err != nil {
		return xerrors.Errorf("exec select: %w", err)
	}
	d.logger.Info(d.logCtx, "acquired database migration lock", slog.F("waited", time.Since(start)))
	return nil
}

func (d *pgTxnDriver) Unlock() error {
	err := d.tx.Commit()
	d.tx = nil
	d.commitErr = err
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

// SetVersion is called by golang-migrate with dirty=true immediately before
// running a migration and with dirty=false once it succeeds, which makes it
// the hook for per-migration progress logging.
//
//nolint:revive
func (d *pgTxnDriver) SetVersion(version int, dirty bool) error {
	switch {
	case version < 0:
	case dirty:
		d.inFlight = version
		d.inFlightStart = time.Now()
		d.logger.Info(d.logCtx, "starting database migration",
			slog.F("version", version),
			slog.F("name", d.migrationName(version)),
		)
	case d.inFlight == version:
		d.logger.Info(d.logCtx, "database migration applied, pending commit",
			slog.F("version", version),
			slog.F("name", d.migrationName(version)),
			slog.F("duration", time.Since(d.inFlightStart)),
		)
		d.applied = append(d.applied, version)
		d.inFlight = -1
	}

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

	return nil
}

// migrationName returns the identifier of the up migration for version, for
// example "add_users_table" for 000002_add_users_table.up.sql.
func (d *pgTxnDriver) migrationName(version int) string {
	if d.source == nil {
		return ""
	}
	r, name, err := d.source.ReadUp(uint(version)) //nolint:gosec // Callers only pass non-negative versions.
	if err != nil {
		return ""
	}
	_ = r.Close()
	return name
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
