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

// TestMigration000634QueuedMessagesConcurrentTraffic runs migration 000634
// in both directions while other connections use chats and
// chat_queued_messages the way chatd does. Each case blocks the migration
// on an open application transaction, waits until pg_blocking_pids shows
// the migration waiting on it, then lets the application continue.
func TestMigration000634QueuedMessagesConcurrentTraffic(t *testing.T) {
	t.Parallel()

	// Every chatstate transition reads the queue. New readers must not
	// wait behind the migration while it waits for an open application
	// transaction.
	t.Run("UpReadQueueWhileMigrationWaits", func(t *testing.T) {
		t.Parallel()
		sqlDB := testSQLDB(t)
		stepTo(t, sqlDB, 633)
		migrationSQL, err := os.ReadFile("000634_chat_queued_messages_editing_since.up.sql")
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitLong)
		chatID, userID := seedChatWithStatus(ctx, t, sqlDB, "waiting")

		// chatd queues a message on a locked chat. The insert trigger also
		// bumps chats.queue_version.
		conn, appPID := dedicatedConn(ctx, t, sqlDB)
		tx, err := conn.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.ExecContext(ctx, `SELECT id FROM chats WHERE id = $1 FOR UPDATE`, chatID)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO chat_queued_messages (chat_id, content, created_by)
			VALUES ($1, '[]', $2)`, chatID, userID)
		require.NoError(t, err)

		pid, migrated := startRolledBackMigration(ctx, t, sqlDB, migrationSQL)
		requireWaitsFor(ctx, t, sqlDB, pid, appPID)

		// IntervalSlow is below the 5s lock_timeout startRolledBackMigration
		// sets, so a stalled reader fails here before the timeout frees it.
		reader, _ := dedicatedConn(ctx, t, sqlDB)
		read := async(func() error {
			var n int
			return reader.QueryRowContext(ctx,
				`SELECT count(*) FROM chat_queued_messages WHERE chat_id = $1`, chatID).Scan(&n)
		})
		readCtx, cancel := context.WithTimeout(ctx, testutil.IntervalSlow)
		defer cancel()
		select {
		case err := <-read:
			require.NoError(t, err, "reader")
		case <-readCtx.Done():
			t.Fatalf("reader of chat_queued_messages did not finish within %s while the application transaction was open", testutil.IntervalSlow)
		}
		select {
		case err := <-migrated:
			t.Fatalf("migration ended before the application transaction ended: %v", err)
		default:
		}

		requireNoDeadlockVictim(ctx, t, "application", async(func() error {
			_, err := tx.ExecContext(ctx, `DELETE FROM chat_queued_messages WHERE chat_id = $1`, chatID)
			if err != nil {
				return err
			}
			return tx.Commit()
		}))
		requireNoDeadlockVictim(ctx, t, "migration", migrated)
	})

	// chatd locks a chat row before its queued rows, so the down migration
	// must not hold the queue while it waits for the chat row.
	t.Run("DownLockChatThenTouchQueue", func(t *testing.T) {
		t.Parallel()
		ctx, sqlDB, migrationSQL, chatID, _ := setupEditingSinceDown(t)

		conn, appPID := dedicatedConn(ctx, t, sqlDB)
		tx, err := conn.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.ExecContext(ctx, `SELECT id FROM chats WHERE id = $1 FOR UPDATE`, chatID)
		require.NoError(t, err)

		pid, migrated := startRolledBackMigration(ctx, t, sqlDB, migrationSQL)
		requireWaitsFor(ctx, t, sqlDB, pid, appPID)

		// The down migration keeps chats_expanded, so a reader of the view
		// must get through while the migration waits for chats.
		reader, _ := dedicatedConn(ctx, t, sqlDB)
		requireNoDeadlockVictim(ctx, t, "reader", async(func() error {
			var id uuid.UUID
			return reader.QueryRowContext(ctx, `SELECT id FROM chats_expanded WHERE id = $1`, chatID).Scan(&id)
		}))

		// Promote the queued head: delete it, then update the chat.
		requireNoDeadlockVictim(ctx, t, "application", async(func() error {
			_, err := tx.ExecContext(ctx, `DELETE FROM chat_queued_messages WHERE chat_id = $1`, chatID)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `UPDATE chats SET status = 'running' WHERE id = $1`, chatID)
			if err != nil {
				return err
			}
			return tx.Commit()
		}))
		requireNoDeadlockVictim(ctx, t, "migration", migrated)
	})

	// An application transaction that has read the queue but holds no chats
	// lock, then queues a message and updates the chat. The insert locks
	// chats through its foreign key check and its queue_version trigger. If
	// the migration takes chats and then waits for the queue, that insert
	// closes a deadlock cycle.
	t.Run("DownReadQueueThenQueueMessage", func(t *testing.T) {
		t.Parallel()
		ctx, sqlDB, migrationSQL, chatID, userID := setupEditingSinceDown(t)

		conn, appPID := dedicatedConn(ctx, t, sqlDB)
		tx, err := conn.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.ExecContext(ctx, `SELECT id FROM chat_queued_messages WHERE chat_id = $1`, chatID)
		require.NoError(t, err)

		pid, migrated := startRolledBackMigration(ctx, t, sqlDB, migrationSQL)
		requireWaitsFor(ctx, t, sqlDB, pid, appPID)

		requireNoDeadlockVictim(ctx, t, "application", async(func() error {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO chat_queued_messages (chat_id, content, created_by)
				VALUES ($1, '[]', $2)`, chatID, userID)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `UPDATE chats SET status = 'running' WHERE id = $1`, chatID)
			if err != nil {
				return err
			}
			return tx.Commit()
		}))
		requireNoDeadlockVictim(ctx, t, "migration", migrated)
	})
}

// setupEditingSinceDown steps a new database to 000634, seeds a paused
// chat with one queued message, and returns the down migration with the
// chat and owner IDs.
func setupEditingSinceDown(t *testing.T) (ctx context.Context, sqlDB *sql.DB, migrationSQL []byte, chatID, userID uuid.UUID) {
	t.Helper()
	sqlDB = testSQLDB(t)
	stepTo(t, sqlDB, 634)
	migrationSQL, err := os.ReadFile("000634_chat_queued_messages_editing_since.down.sql")
	require.NoError(t, err)

	ctx = testutil.Context(t, testutil.WaitLong)
	chatID, userID = seedChatWithStatus(ctx, t, sqlDB, "paused")
	_, err = sqlDB.ExecContext(ctx, `
		INSERT INTO chat_queued_messages (chat_id, content, created_by)
		VALUES ($1, '[]', $2)`, chatID, userID)
	require.NoError(t, err)
	return ctx, sqlDB, migrationSQL, chatID, userID
}

// seedChatWithStatus creates a chat with the given status, owned by a new
// user, and returns the chat and user IDs.
func seedChatWithStatus(ctx context.Context, t *testing.T, db *sql.DB, status string) (chatID, userID uuid.UUID) {
	t.Helper()
	orgID, modelID := uuid.New(), uuid.New()
	chatID, userID = uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, name, display_name, description, created_at, updated_at, default_org_member_roles)
			VALUES ($1, 'org-queue', 'org-queue', '', now(), now(), '{}')`, []any{orgID}},
		{`INSERT INTO users (id, email, username, hashed_password, created_at, updated_at)
			VALUES ($1, 'queue@example.com', 'queue', '\x', now(), now())`, []any{userID}},
		{`INSERT INTO chat_model_configs (id, organization_id, model, context_limit, compression_threshold, deleted)
			VALUES ($1, $2, 'model-queue', 1000, 50, true)`, []any{modelID, orgID}},
		{`INSERT INTO chats (id, owner_id, organization_id, last_model_config_id, title, status)
			VALUES ($1, $2, $3, $4, 'chat-queue', $5)`, []any{chatID, userID, orgID, modelID, status}},
	} {
		_, err := db.ExecContext(ctx, q.sql, q.args...)
		require.NoError(t, err)
	}
	return chatID, userID
}

// startRolledBackMigration runs migrationSQL in a transaction on its own
// connection and rolls it back. It returns the backend PID and the
// migration's result, sent after the rollback.
func startRolledBackMigration(ctx context.Context, t *testing.T, db *sql.DB, migrationSQL []byte) (int, <-chan error) {
	t.Helper()
	conn, pid := dedicatedConn(ctx, t, db)
	return pid, async(func() error {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		// The migration lowers lock_timeout while it takes its locks and
		// must restore the caller's setting afterwards.
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

// requireWaitsFor waits until pid waits for a lock held or requested by
// blocker.
func requireWaitsFor(ctx context.Context, t *testing.T, db *sql.DB, pid, blocker int) {
	t.Helper()
	require.Eventually(t, func() bool {
		var blocked bool
		err := db.QueryRowContext(ctx, `SELECT $2::int = ANY(pg_blocking_pids($1))`, pid, blocker).Scan(&blocked)
		return err == nil && blocked
	}, testutil.WaitMedium, testutil.IntervalFast, "pid %d never waited for pid %d", pid, blocker)
}

// requireNoDeadlockVictim receives the result from done and fails the
// test with the deadlock detail if PostgreSQL chose that side as a
// deadlock victim.
func requireNoDeadlockVictim(ctx context.Context, t *testing.T, who string, done <-chan error) {
	t.Helper()
	err := testutil.TryReceive(ctx, t, done)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "40P01" {
		t.Fatalf("%s: %v: %s", who, err, pqErr.Detail)
	}
	require.NoError(t, err, who)
}
