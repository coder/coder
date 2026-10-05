package migrations_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// TestMigration000612QueuedMessagesConcurrentTraffic runs migration 000612
// in both directions while other connections use chats and
// chat_queued_messages the way chatd does. See
// TestMigrationChatTablesConcurrentTraffic for the approach.
func TestMigration000612QueuedMessagesConcurrentTraffic(t *testing.T) {
	t.Parallel()

	// Every chatstate transition reads the queue. While the migration waits
	// behind an open application transaction, new readers of
	// chat_queued_messages must not queue behind it until that transaction
	// ends.
	t.Run("UpReadQueueWhileMigrationWaits", func(t *testing.T) {
		t.Parallel()
		sqlDB := testSQLDB(t)
		stepTo(t, sqlDB, 611)
		migrationSQL, err := os.ReadFile("000612_chat_queued_messages_editing_since.up.sql")
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitLong)
		chatID, userID := seedChat(ctx, t, sqlDB, "waiting")

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

		pid, migrated := startMigration(ctx, t, sqlDB, migrationSQL)
		requireBlockedBy(ctx, t, sqlDB, pid, appPID)

		reader, _ := dedicatedConn(ctx, t, sqlDB)
		requireNoDeadlock(ctx, t, "reader", async(func() error {
			var n int
			return reader.QueryRowContext(ctx,
				`SELECT count(*) FROM chat_queued_messages WHERE chat_id = $1`, chatID).Scan(&n)
		}))
		select {
		case err := <-migrated:
			t.Fatalf("migration ended before the application transaction ended: %v", err)
		default:
		}

		requireNoDeadlock(ctx, t, "application", async(func() error {
			_, err := tx.ExecContext(ctx, `DELETE FROM chat_queued_messages WHERE chat_id = $1`, chatID)
			if err != nil {
				return err
			}
			return tx.Commit()
		}))
		requireNoDeadlock(ctx, t, "migration", migrated)
	})

	// chatd locks a chat row before touching its queued rows. The down
	// migration changes chat_queued_messages and then updates paused chats,
	// so it must not hold the queue while it waits for the chat row.
	t.Run("DownLockChatThenTouchQueue", func(t *testing.T) {
		t.Parallel()
		sqlDB := testSQLDB(t)
		stepTo(t, sqlDB, 612)
		migrationSQL, err := os.ReadFile("000612_chat_queued_messages_editing_since.down.sql")
		require.NoError(t, err)

		ctx := testutil.Context(t, testutil.WaitLong)
		chatID, userID := seedChat(ctx, t, sqlDB, "paused")
		_, err = sqlDB.ExecContext(ctx, `
			INSERT INTO chat_queued_messages (chat_id, content, created_by)
			VALUES ($1, '[]', $2)`, chatID, userID)
		require.NoError(t, err)

		conn, appPID := dedicatedConn(ctx, t, sqlDB)
		tx, err := conn.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.ExecContext(ctx, `SELECT id FROM chats WHERE id = $1 FOR UPDATE`, chatID)
		require.NoError(t, err)

		pid, migrated := startMigration(ctx, t, sqlDB, migrationSQL)
		requireBlockedBy(ctx, t, sqlDB, pid, appPID)

		// The down migration keeps chats_expanded, so a reader of the view
		// must get through while the migration waits for chats.
		reader, _ := dedicatedConn(ctx, t, sqlDB)
		requireNoDeadlock(ctx, t, "reader", async(func() error {
			var id uuid.UUID
			return reader.QueryRowContext(ctx, `SELECT id FROM chats_expanded WHERE id = $1`, chatID).Scan(&id)
		}))

		// Promote the queued head: delete it, then update the chat.
		requireNoDeadlock(ctx, t, "application", async(func() error {
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
		requireNoDeadlock(ctx, t, "migration", migrated)
	})
}

// seedChat creates a chat with the given status, owned by a new user, and
// returns the chat and user IDs.
func seedChat(ctx context.Context, t *testing.T, db *sql.DB, status string) (chatID, userID uuid.UUID) {
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
