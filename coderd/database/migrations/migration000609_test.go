package migrations_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/testutil"
)

// TestMigrationChatTablesConcurrentTraffic runs the migrations that alter
// chats and recreate chats_expanded while other connections use those
// relations the way chatd does, and checks that neither side deadlocks.
// Each case blocks the migration on an open application transaction, waits
// until pg_blocking_pids shows the migration waiting on it, then lets the
// application continue.
//
//nolint:tparallel,paralleltest // Cases share one database; each migration attempt rolls back.
func TestMigrationChatTablesConcurrentTraffic(t *testing.T) {
	t.Parallel()

	for _, m := range []struct {
		from uint
		file string
	}{
		{from: 608, file: "000609_chat_automations.up.sql"},
		{from: 610, file: "000611_chat_manage_automations_enabled.up.sql"},
	} {
		t.Run(m.file, func(t *testing.T) {
			t.Parallel()
			testChatMigrationConcurrentTraffic(t, m.from, m.file)
		})
	}
}

//nolint:tparallel,paralleltest // Cases share one database; each migration attempt rolls back.
func testChatMigrationConcurrentTraffic(t *testing.T, from uint, file string) {
	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, from)

	migrationSQL, err := os.ReadFile(file)
	require.NoError(t, err)

	setupCtx := testutil.Context(t, testutil.WaitLong)
	orgID, userID, modelID, chatID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, name, display_name, description, created_at, updated_at, default_org_member_roles)
			VALUES ($1, 'org-concurrent', 'org-concurrent', '', now(), now(), '{}')`, []any{orgID}},
		{`INSERT INTO users (id, email, username, hashed_password, created_at, updated_at)
			VALUES ($1, 'concurrent@example.com', 'concurrent', '\x', now(), now())`, []any{userID}},
		{`INSERT INTO chat_model_configs (id, organization_id, model, context_limit, compression_threshold, deleted)
			VALUES ($1, $2, 'model-concurrent', 1000, 50, true)`, []any{modelID, orgID}},
		{`INSERT INTO chats (id, owner_id, organization_id, last_model_config_id, title)
			VALUES ($1, $2, $3, $4, 'chat-concurrent')`, []any{chatID, userID, orgID, modelID}},
	} {
		_, err = sqlDB.ExecContext(setupCtx, q.sql, q.args...)
		require.NoError(t, err)
	}

	// startMigration runs the migration in a transaction on its own
	// connection and rolls it back, so the next case starts from the same
	// version. It returns the backend PID and the migration's result, sent
	// after the rollback.
	startMigration := func(t *testing.T, ctx context.Context) (int, <-chan error) {
		t.Helper()
		conn, pid := dedicatedConn(ctx, t, sqlDB)
		return pid, async(func() error {
			tx, err := conn.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			// The migration lowers lock_timeout while it takes its locks
			// and must restore the caller's setting afterwards.
			_, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout = '5s'`)
			if err == nil {
				_, err = tx.ExecContext(ctx, string(migrationSQL))
			}
			if err == nil {
				var lockTimeout string
				err = tx.QueryRowContext(ctx, `SHOW lock_timeout`).Scan(&lockTimeout)
				if err == nil && lockTimeout != "5s" {
					err = xerrors.Errorf("lock_timeout after migration = %q, want 5s", lockTimeout)
				}
			}
			return errors.Join(err, tx.Rollback())
		})
	}
	// isBlockedBy reports whether pid waits for a lock held or requested by
	// blocker.
	isBlockedBy := func(ctx context.Context, pid, blocker int) bool {
		var blocked bool
		err := sqlDB.QueryRowContext(ctx, `SELECT $2::int = ANY(pg_blocking_pids($1))`, pid, blocker).Scan(&blocked)
		return err == nil && blocked
	}
	requireBlockedBy := func(t *testing.T, ctx context.Context, pid, blocker int) {
		t.Helper()
		require.Eventually(t, func() bool { return isBlockedBy(ctx, pid, blocker) },
			testutil.WaitMedium, testutil.IntervalFast, "pid %d never waited for pid %d", pid, blocker)
	}
	requireNoDeadlock := func(t *testing.T, ctx context.Context, who string, done <-chan error) {
		t.Helper()
		err := testutil.TryReceive(ctx, t, done)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "40P01" {
			t.Fatalf("%s: %v: %s", who, err, pqErr.Detail)
		}
		require.NoError(t, err, who)
	}
	// lockChat opens an application transaction that holds a row lock on
	// the chat, as GetChatByIDForUpdate does.
	lockChat := func(t *testing.T, ctx context.Context) (*sql.Tx, int) {
		t.Helper()
		conn, pid := dedicatedConn(ctx, t, sqlDB)
		tx, err := conn.BeginTx(ctx, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = tx.Rollback() })
		_, err = tx.ExecContext(ctx, `SELECT id FROM chats WHERE id = $1 FOR UPDATE`, chatID)
		require.NoError(t, err)
		return tx, pid
	}
	updateAndCommit := func(ctx context.Context, tx *sql.Tx) func() error {
		return func() error {
			_, err := tx.ExecContext(ctx, `UPDATE chats SET title = 'renamed' WHERE id = $1`, chatID)
			if err != nil {
				return err
			}
			return tx.Commit()
		}
	}

	// chatd locks a chat row, then updates the chat. The migration must not
	// hold a lock on chats that blocks the update while it waits for the row
	// lock to go away.
	t.Run("LockChatThenUpdate", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		tx, appPID := lockChat(t, ctx)

		pid, migrated := startMigration(t, ctx)
		requireBlockedBy(t, ctx, pid, appPID)

		requireNoDeadlock(t, ctx, "application", async(updateAndCommit(ctx, tx)))
		requireNoDeadlock(t, ctx, "migration", migrated)
	})

	// chatd inserts a message, whose triggers read and update chats, then
	// updates the chat.
	t.Run("InsertMessageThenUpdateChat", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		conn, appPID := dedicatedConn(ctx, t, sqlDB)
		tx, err := conn.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chat_messages (chat_id, role, content_version)
			VALUES ($1, 'user', 1)`, chatID)
		require.NoError(t, err)

		pid, migrated := startMigration(t, ctx)
		requireBlockedBy(t, ctx, pid, appPID)

		requireNoDeadlock(t, ctx, "application", async(updateAndCommit(ctx, tx)))
		requireNoDeadlock(t, ctx, "migration", migrated)
	})

	// GetChatByID reads chats_expanded, which locks the view before chats.
	// A reader that arrives while the migration waits for chats must not end
	// up holding the view lock the migration needs later.
	t.Run("ReadViewWhileMigrationWaits", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		tx, appPID := lockChat(t, ctx)

		pid, migrated := startMigration(t, ctx)
		requireBlockedBy(t, ctx, pid, appPID)

		reader, readerPID := dedicatedConn(ctx, t, sqlDB)
		read := async(func() error {
			var id uuid.UUID
			return reader.QueryRowContext(ctx, `SELECT id FROM chats_expanded WHERE id = $1`, chatID).Scan(&id)
		})
		// The fixed migration retries its locks, so the reader may finish
		// without ever waiting. Either way, it must have started before the
		// application commits.
		require.Eventually(t, func() bool {
			return len(read) > 0 || isBlockedBy(ctx, readerPID, pid)
		}, testutil.WaitMedium, testutil.IntervalFast, "reader neither finished nor waited for the migration")

		require.NoError(t, tx.Commit())
		requireNoDeadlock(t, ctx, "reader", read)
		requireNoDeadlock(t, ctx, "migration", migrated)
	})

	// A transaction that already locked a chat reads chats_expanded, as
	// dbauthz does when it fetches a chat it is about to update.
	t.Run("LockChatThenReadView", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		tx, appPID := lockChat(t, ctx)

		pid, migrated := startMigration(t, ctx)
		requireBlockedBy(t, ctx, pid, appPID)

		read := async(func() error {
			var id uuid.UUID
			if err := tx.QueryRowContext(ctx, `SELECT id FROM chats_expanded WHERE id = $1`, chatID).Scan(&id); err != nil {
				return err
			}
			return tx.Commit()
		})
		requireNoDeadlock(t, ctx, "application", read)
		requireNoDeadlock(t, ctx, "migration", migrated)
	})
}

// async runs fn in a goroutine and returns a channel that receives its
// result.
func async(fn func() error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	return done
}

// dedicatedConn returns a connection that stays checked out for the test and
// its backend PID.
func dedicatedConn(ctx context.Context, t *testing.T, db *sql.DB) (*sql.Conn, int) {
	t.Helper()
	conn, err := db.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	var pid int
	require.NoError(t, conn.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
	return conn, pid
}
