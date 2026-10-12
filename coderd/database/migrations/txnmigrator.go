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
// chatd keeps using chats, the tables that reference it (chat_messages,
// chat_queued_messages, chat_automations and others) and the chats_expanded
// view while a rolling deploy migrates the database, and it locks chats and
// chats_expanded in both orders. Migrations that alter those
// relations one after another (for example 000600, 000601, 000607 and 000608)
// take and upgrade their locks in an order that can deadlock with that
// traffic. Taking every lock up front, with short waits that are retried, is
// the approach of 000609_chat_automations.up.sql, applied once to the whole
// migration transaction:
//
//   - On a fresh database chats does not exist and nothing uses it, so there
//     is nothing to lock.
//   - Each attempt ends within half of deadlock_timeout: every lock wait is
//     capped at 100ms and at the time the attempt has left. PostgreSQL runs
//     the deadlock check in the backend that has waited deadlock_timeout and
//     aborts that backend. This transaction always gives up first, so it
//     never runs the check, and an application transaction never waits for
//     it longer than one attempt. A wait that times out rolls back the
//     subtransaction, which releases every lock the attempt took, and the
//     attempt starts again.
//   - Attempts alternate between locking the chats_expanded view before and
//     after the tables. Readers of the view lock it before chats, while
//     transactions that lock a chat row and then read the view lock chats
//     first. Under sustained traffic of one kind, every attempt in the
//     opposite order times out, so a fixed order can fail to get the locks
//     at all.
//   - ALTER VIEW with the view's current options is a no-op that locks only
//     the view. LOCK TABLE on the view would also lock chats,
//     visible_users and users in ACCESS EXCLUSIVE mode, which blocks all API
//     authentication until the migrations commit. No migration sets
//     check_option or security_barrier on chats_expanded, so the RESET form
//     runs today. The SET form keeps any future options unchanged.
//   - Every table with a foreign key path to chats is locked too, found from
//     pg_constraint so that tables added later are covered. An application
//     write to such a table locks the table, then waits for chats in its
//     foreign key check while this transaction holds chats. A later
//     migration that locks the table would then deadlock with it.
//   - Only these relations are locked here. Later migrations still wait
//     for their other locks without a timeout, while this transaction holds
//     the chat locks. An application transaction that holds a lock on a
//     table a later migration alters, and then touches a chat table, can
//     deadlock with it. If the application touches the chat table less than
//     deadlock_timeout after the migration started waiting, PostgreSQL
//     aborts the migrations and UpWithFS retries them. Otherwise it aborts
//     the application transaction. Locking more tables here, such as users
//     for the foreign keys of 000609, would block all their writes until
//     the migrations commit.
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
	-- Every lock an attempt takes is held until the attempt ends, so the
	-- whole attempt, not each lock wait, must stay below deadlock_timeout.
	attempt_budget interval := current_setting('deadlock_timeout')::interval / 2;
	lock_wait interval := least(interval '100ms', attempt_budget);
	attempt_deadline timestamptz;
	remaining interval;
	lock_tables text[];
	lock_view text;
	statements text[];
	statement text;
	view_first boolean := false;
	view_options text[];
BEGIN
	IF to_regclass('chats') IS NULL THEN
		RETURN;
	END IF;
	-- chats and every table whose foreign keys lead to it, directly or
	-- through another such table. chats comes first.
	WITH RECURSIVE locked(oid) AS (
		SELECT to_regclass('chats')::oid
		UNION
		SELECT con.conrelid
		FROM pg_constraint con
		JOIN locked ON con.confrelid = locked.oid
		WHERE con.contype = 'f'
	)
	SELECT array_agg(format('LOCK TABLE %I.%I IN ACCESS EXCLUSIVE MODE', n.nspname, c.relname)
		ORDER BY c.relname <> 'chats', c.relname)
	INTO lock_tables
	FROM locked
	JOIN pg_class c ON c.oid = locked.oid
	JOIN pg_namespace n ON n.oid = c.relnamespace;
	IF to_regclass('chats_expanded') IS NOT NULL THEN
		SELECT reloptions INTO view_options FROM pg_class WHERE oid = to_regclass('chats_expanded');
		IF view_options IS NULL THEN
			lock_view := 'ALTER VIEW chats_expanded RESET (check_option)';
		ELSE
			lock_view := format('ALTER VIEW chats_expanded SET (%s)', array_to_string(view_options, ', '));
		END IF;
	END IF;
	LOOP
		view_first := NOT view_first;
		statements := lock_tables;
		IF lock_view IS NOT NULL AND view_first THEN
			statements := lock_view || statements;
		ELSIF lock_view IS NOT NULL THEN
			statements := statements || lock_view;
		END IF;
		BEGIN
			attempt_deadline := clock_timestamp() + attempt_budget;
			FOREACH statement IN ARRAY statements LOOP
				remaining := attempt_deadline - clock_timestamp();
				IF remaining <= interval '0' THEN
					RAISE EXCEPTION 'chat table lock attempt ran out of time' USING ERRCODE = 'lock_not_available';
				END IF;
				PERFORM set_config('lock_timeout',
					greatest(1, floor(extract(epoch FROM least(lock_wait, remaining)) * 1000))::bigint || 'ms', true);
				EXECUTE statement;
			END LOOP;
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
		// Not runStatement: its error repeats the whole query, which hides
		// the message.
		if _, err := d.tx.ExecContext(d.ctx, lockChatTablesQuery); err != nil {
			return xerrors.Errorf("lock chat tables before running migrations: %w", err)
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
