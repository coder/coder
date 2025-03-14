package migrations
import (
	"errors"
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/lib/pq"
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
	const q = `
SELECT pg_advisory_xact_lock($1)
`
	_, err = d.tx.ExecContext(d.ctx, q, lockID)
	if err != nil {
		return fmt.Errorf("exec select: %w", err)
	}
	return nil
}
func (d *pgTxnDriver) Unlock() error {
	err := d.tx.Commit()
	d.tx = nil
	if err != nil {
		return fmt.Errorf("commit tx on unlock: %w", err)
	}
	return nil
}
func (d *pgTxnDriver) Run(migration io.Reader) error {
	migr, err := io.ReadAll(migration)
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}
	err = d.runStatement(migr)
	if err != nil {
		return fmt.Errorf("run statement: %w", err)
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
		if errors.As(err, &pgErr) {
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
		return database.NilVersion, false, nil
	case err != nil:
		var pgErr *pq.Error
		if errors.As(err, &pgErr) {
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
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	const query = `CREATE TABLE IF NOT EXISTS ` + migrationsTableName + ` (version bigint not null primary key, dirty boolean not null)`
	if _, err := d.tx.ExecContext(context.Background(), query); err != nil {
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}
	err = d.Unlock()
	if err != nil {
		return fmt.Errorf("release migration lock: %w", err)
	}
	return nil
}
