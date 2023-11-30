package migrations

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/lib/pq"
	"golang.org/x/xerrors"
)

const (
	lockID              = int64(1037453835920848937)
	migrationsTableName = "schema_migrations"
)

func betterSetup(db *sql.DB) (source.Driver, *migrate.Migrate, error) {
	ctx := context.Background()
	sourceDriver, err := iofs.New(migrations, ".")
	if err != nil {
		return nil, nil, xerrors.Errorf("create iofs: %w", err)
	}

	// migration_cursor is a v1 migration table. If this exists, we're on v1.
	// Do no run v2 migrations on a v1 database!
	row := db.QueryRowContext(ctx, "SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'migration_cursor';")
	var v1Exists int
	if row.Scan(&v1Exists) == nil {
		return nil, nil, xerrors.New("currently connected to a Coder v1 database, aborting database setup")
	}

	dbDriver := &pgTxnDriver{ctx: context.Background(), db: db}
	err = dbDriver.ensureVersionTable()
	if err != nil {
		return nil, nil, xerrors.Errorf("ensure version table: %w", err)
	}

	m, err := migrate.NewWithInstance("", sourceDriver, "", dbDriver)
	if err != nil {
		return nil, nil, xerrors.Errorf("new migrate instance: %w", err)
	}

	return sourceDriver, m, nil
}

type pgTxnDriver struct {
	ctx context.Context
	db  *sql.DB
	tx  *sql.Tx
}

func (d *pgTxnDriver) Open(url string) (database.Driver, error) {
	panic("not implemented")
}

func (d *pgTxnDriver) Close() error {
	return nil
}

func (d *pgTxnDriver) Lock() error {
	var err error

	d.tx, err = d.db.BeginTx(d.ctx, nil)
	if err != nil {
		return err
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

func (d *pgTxnDriver) Unlock() error {
	err := d.tx.Commit()
	d.tx = nil
	if err != nil {
		return xerrors.Errorf("commit tx on unlock: %w", err)
	}
	return nil
}

// Run applies a migration to the database. migration is guaranteed to be not nil.
func (d *pgTxnDriver) Run(migration io.Reader) error {
	migr, err := io.ReadAll(migration)
	if err != nil {
		return xerrors.Errorf("read migration: %w", err)
	}
	migr = bytes.ReplaceAll(migr, []byte("BEGIN;"), []byte{})
	migr = bytes.ReplaceAll(migr, []byte("COMMIT;"), []byte{})
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
		if pgErr, ok := err.(*pq.Error); ok {
			var line uint
			message := fmt.Sprintf("migration failed: %s", pgErr.Message)
			if pgErr.Detail != "" {
				message = fmt.Sprintf("%s, %s", message, pgErr.Detail)
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

	return nil
}

func (d *pgTxnDriver) Version() (version int, dirty bool, err error) {
	var q interface {
		QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	} = d.tx
	if d.tx == nil {
		q = d.db
	}

	query := `SELECT version, dirty FROM ` + migrationsTableName + ` LIMIT 1`
	err = q.QueryRowContext(context.Background(), query).Scan(&version, &dirty)
	switch {
	case err == sql.ErrNoRows:
		return database.NilVersion, false, nil

	case err != nil:
		if e, ok := err.(*pq.Error); ok {
			if e.Code.Name() == "undefined_table" {
				return database.NilVersion, false, nil
			}
		}
		return 0, false, &database.Error{OrigErr: err, Query: []byte(query)}

	default:
		return version, dirty, nil
	}
}

func (d *pgTxnDriver) Drop() error {
	panic("not implemented")
}

func (d *pgTxnDriver) ensureVersionTable() error {
	const query = `CREATE TABLE IF NOT EXISTS ` + migrationsTableName + ` (version bigint not null primary key, dirty boolean not null)`
	if _, err := d.db.ExecContext(context.Background(), query); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}

	return nil
}
