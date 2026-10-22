package migrations_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// TestMigration000609UpgradeFromReleaseDuringChatTraffic upgrades a v2.38
// database (migration 605) while chatd writes chat messages. coder server
// runs every pending migration in one transaction, so the lock that 000607
// and 000608 take on chats is held until the upgrade commits. Each insert
// takes its lock on chat_messages and then waits for chats, so it holds a
// lock that 000609 needs. Postgres can resolve this by canceling the inserts
// as deadlock victims, and the upgrade must then complete. A migration that
// retries short lock waits instead does not get chat_messages, and the
// upgrade fails.
func TestMigration000609UpgradeFromReleaseDuringChatTraffic(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 605)

	ctx := testutil.Context(t, testutil.WaitLong)
	orgID, userID, modelID, chatID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, name, display_name, description, created_at, updated_at, default_org_member_roles)
			VALUES ($1, 'org-609', 'org-609', '', now(), now(), '{}')`, []any{orgID}},
		{`INSERT INTO users (id, email, username, hashed_password, created_at, updated_at)
			VALUES ($1, 'user609@example.com', 'user609', '\x', now(), now())`, []any{userID}},
		{`INSERT INTO chat_model_configs (id, organization_id, model, context_limit, compression_threshold, deleted)
			VALUES ($1, $2, 'model-609', 1000, 50, true)`, []any{modelID, orgID}},
		{`INSERT INTO chats (id, owner_id, organization_id, last_model_config_id, title)
			VALUES ($1, $2, $3, $4, 'chat-609')`, []any{chatID, userID, orgID, modelID}},
	} {
		_, err := sqlDB.ExecContext(ctx, q.sql, q.args...)
		require.NoError(t, err)
	}

	migrationConn, migrationPID := dedicatedConn(ctx, t, sqlDB)
	tx, err := migrationConn.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	for version := 606; version <= 608; version++ {
		_, err := tx.ExecContext(ctx, readUpMigration(t, version))
		require.NoError(t, err, "migration %d", version)
	}

	// Postgres checks a waiting insert for a deadlock once, deadlock_timeout
	// after it starts waiting, and finds none if the migration is not waiting
	// at that moment. Staggered timeouts spread those checks over 150ms, so a
	// migration that pauses between short lock waits is paused for at least
	// one of them. Every check fires after 000609 starts waiting and before
	// its own check at the default 1s, so Postgres cancels inserts rather
	// than the migration.
	var inserted []<-chan error
	for i := range 4 {
		appConn, appPID := dedicatedConn(ctx, t, sqlDB)
		_, err := appConn.ExecContext(ctx, fmt.Sprintf("SET deadlock_timeout = '%dms'", 500+50*i))
		require.NoError(t, err)
		inserted = append(inserted, async(func() error {
			_, err := appConn.ExecContext(ctx, `INSERT INTO chat_messages (chat_id, role, content_version) VALUES ($1, 'user', 1)`, chatID)
			return err
		}))
		require.Eventually(t, func() bool {
			var blocked bool
			err := sqlDB.QueryRowContext(ctx, `SELECT $2::int = ANY(pg_blocking_pids($1))`, appPID, migrationPID).Scan(&blocked)
			return err == nil && blocked
		}, testutil.WaitMedium, testutil.IntervalFast, "the insert never waited for the upgrade")
	}

	migration609 := readUpMigration(t, 609)
	migrated := async(func() error {
		_, err := tx.ExecContext(ctx, migration609)
		return err
	})
	require.NoError(t, testutil.TryReceive(ctx, t, migrated), "migration 609")
	require.NoError(t, tx.Commit())

	for _, done := range inserted {
		if err := testutil.TryReceive(ctx, t, done); err != nil {
			var pqErr *pq.Error
			require.Truef(t, errors.As(err, &pqErr) && pqErr.Code == "40P01", "insert failed for a reason other than a deadlock: %v", err)
		}
	}
}

func readUpMigration(t *testing.T, version int) string {
	t.Helper()
	files, err := filepath.Glob(fmt.Sprintf("%06d_*.up.sql", version))
	require.NoError(t, err)
	require.Len(t, files, 1, "up migration %d", version)
	query, err := os.ReadFile(files[0])
	require.NoError(t, err)
	return string(query)
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
