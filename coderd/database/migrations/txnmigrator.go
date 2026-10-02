package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"

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
	// chatTablesLocked reports whether tx already ran lockChatTablesQuery.
	chatTablesLocked bool
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
	d.chatTablesLocked = false
	const q = `
SELECT pg_advisory_xact_lock($1)
`

	_, err = d.tx.ExecContext(d.ctx, q, lockID)
	if err != nil {
		// golang-migrate does not call Unlock after a failed Lock. Roll
		// back here so a retry in UpWithFS does not leave a connection
		// stuck in an open transaction.
		_ = d.tx.Rollback()
		d.tx = nil
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

// lockChatTablesQuery takes the chat table locks that migrations need,
// before any pending migration touches those tables. See Run.
//
// chatd keeps using chats, chat_messages, chat_queued_messages and the
// chats_expanded view while a rolling deploy migrates the database, and it
// locks chats and chats_expanded in both orders. Migrations that alter those
// relations one after another (for example 000600, 000601, 000607 and 000608)
// take and upgrade their locks in an order that can deadlock with that
// traffic. Taking every lock up front, with short waits that are retried, is
// the approach of 000609_chat_automations.up.sql, applied once to the whole
// migration transaction:
//
//   - On a fresh database chats does not exist and nothing uses it, so there
//     is nothing to lock.
//   - lock_timeout stays below deadlock_timeout. PostgreSQL runs the deadlock
//     check in the backend that has waited deadlock_timeout and aborts that
//     backend. This transaction always gives up first, so it never runs the
//     check, and an application transaction waits at most one lock_timeout
//     for it. A wait that times out rolls back the subtransaction, which
//     releases every lock the attempt took, and the attempt starts again.
//   - The chats_expanded view is locked before the tables, as readers of the
//     view do. ALTER VIEW with the view's current options is a no-op that
//     locks only the view. LOCK TABLE on the view would also lock chats,
//     visible_users and users in ACCESS EXCLUSIVE mode, which blocks all API
//     authentication until the migrations commit. No migration sets
//     check_option or security_barrier on chats_expanded, so the RESET form
//     runs today. The SET form keeps any future options unchanged.
//   - The tables that 000609 locks for its foreign keys (organizations,
//     users and chat_model_configs) are not locked here: locking users for
//     every upgrade would block all user writes until the migrations commit.
//     An application transaction that writes one of those tables and then
//     touches a chat table can still deadlock with a later migration that
//     locks them, and PostgreSQL can then abort either side. If it aborts
//     the migrations, UpWithFS retries them.
//
// The locks are held until the migration transaction commits, so chat traffic
// waits for the whole migration run, even when no pending migration touches
// the chat tables. That is the accepted cost: almost every release has a
// migration that alters chats, and such a migration holds ACCESS EXCLUSIVE on
// chats until commit anyway. Deciding from the migration SQL whether a run
// touches the chat tables could miss an indirect reference and bring the
// deadlock back.
const lockChatTablesQuery = `
DO $$
DECLARE
	previous_lock_timeout text := current_setting('lock_timeout');
	deadline timestamptz := clock_timestamp() + interval '2 minutes';
	-- Half of deadlock_timeout, at most 100ms and at least 1ms.
	attempt_lock_timeout text := greatest(1, floor(extract(epoch FROM
		least(interval '100ms', current_setting('deadlock_timeout')::interval / 2)) * 1000))::bigint || 'ms';
	tables text;
	view_options text[];
BEGIN
	IF to_regclass('chats') IS NULL THEN
		RETURN;
	END IF;
	SELECT string_agg(quote_ident(t), ', ') INTO tables
	FROM unnest(ARRAY['chats', 'chat_messages', 'chat_queued_messages']) AS t
	WHERE to_regclass(t) IS NOT NULL;
	LOOP
		BEGIN
			PERFORM set_config('lock_timeout', attempt_lock_timeout, true);
			IF to_regclass('chats_expanded') IS NOT NULL THEN
				SELECT reloptions INTO view_options FROM pg_class WHERE oid = to_regclass('chats_expanded');
				IF view_options IS NULL THEN
					ALTER VIEW chats_expanded RESET (check_option);
				ELSE
					EXECUTE format('ALTER VIEW chats_expanded SET (%s)', array_to_string(view_options, ', '));
				END IF;
			END IF;
			EXECUTE 'LOCK TABLE ' || tables || ' IN ACCESS EXCLUSIVE MODE';
			EXIT;
		EXCEPTION WHEN lock_not_available OR deadlock_detected THEN
			IF clock_timestamp() > deadline THEN
				RAISE EXCEPTION 'migrations could not lock the chat tables within 2 minutes';
			END IF;
		END;
		PERFORM pg_sleep(0.1);
	END LOOP;
	PERFORM set_config('lock_timeout', previous_lock_timeout, true);
END;
$$;
`

func (d *pgTxnDriver) Run(migration io.Reader) error {
	migr, err := io.ReadAll(migration)
	if err != nil {
		return xerrors.Errorf("read migration: %w", err)
	}
	// golang-migrate calls Run only for a pending migration, so the locks
	// are taken only when there is work, and before the first migration
	// body of the transaction.
	if !d.chatTablesLocked {
		err = d.runStatement([]byte(lockChatTablesQuery))
		if err != nil {
			return xerrors.Errorf("lock chat tables: %w", err)
		}
		d.chatTablesLocked = true
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
		// Do not leave the transaction open: UpWithFS can retry, and the
		// retry would wait for the advisory lock this transaction holds.
		_ = d.tx.Rollback()
		d.tx = nil
		return &database.Error{OrigErr: err, Query: []byte(query)}
	}

	err = d.Unlock()
	if err != nil {
		return xerrors.Errorf("release migration lock: %w", err)
	}

	return nil
}
