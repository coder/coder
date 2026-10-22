package migrations_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

// TestMigrationUpgradeFromReleaseDuringChatTraffic upgrades a v2.38 database
// (migration 605) in one transaction, as coder server does, while chat
// requests wait for the lock that 000607 and 000608 hold on chats. Each
// request holds a lock that the target migration needs, so they deadlock.
// Postgres checks a waiting request for a deadlock once, deadlock_timeout
// after it starts waiting. The migration is waiting at that moment, so
// Postgres cancels the request and the upgrade continues. A migration that
// pauses between short lock waits misses some of those checks and is still
// retrying when the test context expires.
//
// This covers only requests that start waiting shortly before the migration
// needs their lock. A request that waited longer than deadlock_timeout has
// already had its check, so Postgres cancels the migration instead and the
// upgrade fails, with or without lock retries.
func TestMigrationUpgradeFromReleaseDuringChatTraffic(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		target int
		// request holds a lock that target needs, then waits for chats.
		request func(ctx context.Context, conn *sql.Conn, chatID, userID uuid.UUID) error
	}{
		{
			name:   "000609ChatMessageInsert",
			target: 609,
			request: func(ctx context.Context, conn *sql.Conn, chatID, _ uuid.UUID) error {
				_, err := conn.ExecContext(ctx, `INSERT INTO chat_messages (chat_id, role, content_version) VALUES ($1, 'user', 1)`, chatID)
				return err
			},
		},
		{
			name:   "000615ReadUserThenChat",
			target: 615,
			request: func(ctx context.Context, conn *sql.Conn, chatID, userID uuid.UUID) error {
				tx, err := conn.BeginTx(ctx, nil)
				if err != nil {
					return err
				}
				defer func() { _ = tx.Rollback() }()
				if _, err := tx.ExecContext(ctx, `SELECT id FROM users WHERE id = $1`, userID); err != nil {
					return err
				}
				_, err = tx.ExecContext(ctx, `SELECT id FROM chats WHERE id = $1`, chatID)
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			testUpgradeFromReleaseDuringChatTraffic(t, tc.target, tc.request)
		})
	}
}

func testUpgradeFromReleaseDuringChatTraffic(t *testing.T, target int, request func(ctx context.Context, conn *sql.Conn, chatID, userID uuid.UUID) error) {
	// Each request gets its own deadlock_timeout, so the checks land at
	// different points of a lock-retry cycle and at least one lands in a
	// pause. requestDeadlockTimeout is much longer than the time between the
	// first request waiting and the migration waiting, so every check finds
	// the migration waiting. migrationDeadlockTimeout keeps the migration's
	// own check from firing first; in production the requests start waiting
	// first and every check uses the same timeout, with the same result.
	const (
		requests                   = 4
		requestDeadlockTimeout     = 2 * time.Second
		requestDeadlockTimeoutStep = 50 * time.Millisecond
		migrationDeadlockTimeout   = 10 * time.Second
	)

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 605)

	ctx := testutil.Context(t, testutil.WaitLong)
	orgID, userID, modelID, chatID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, name, display_name, description, created_at, updated_at, default_org_member_roles)
			VALUES ($1, 'org-upgrade', 'org-upgrade', '', now(), now(), '{}')`, []any{orgID}},
		{`INSERT INTO users (id, email, username, hashed_password, created_at, updated_at)
			VALUES ($1, 'upgrade@example.com', 'upgrade', '\x', now(), now())`, []any{userID}},
		{`INSERT INTO chat_model_configs (id, organization_id, model, context_limit, compression_threshold, deleted)
			VALUES ($1, $2, 'model-upgrade', 1000, 50, true)`, []any{modelID, orgID}},
		{`INSERT INTO chats (id, owner_id, organization_id, last_model_config_id, title)
			VALUES ($1, $2, $3, $4, 'chat-upgrade')`, []any{chatID, userID, orgID, modelID}},
	} {
		_, err := sqlDB.ExecContext(ctx, q.sql, q.args...)
		require.NoError(t, err)
	}

	migrationConn, migrationPID := dedicatedConn(ctx, t, sqlDB)
	tx, err := migrationConn.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL deadlock_timeout = '%dms'", migrationDeadlockTimeout.Milliseconds()))
	require.NoError(t, err)
	for version := 606; version < target; version++ {
		_, err := tx.ExecContext(ctx, readUpMigration(t, version))
		require.NoError(t, err, "migration %d", version)
	}

	var requestResults []<-chan error
	for i := range requests {
		requestConn, requestPID := dedicatedConn(ctx, t, sqlDB)
		timeout := requestDeadlockTimeout + time.Duration(i)*requestDeadlockTimeoutStep
		_, err := requestConn.ExecContext(ctx, fmt.Sprintf("SET deadlock_timeout = '%dms'", timeout.Milliseconds()))
		require.NoError(t, err)
		requestResults = append(requestResults, async(func() error {
			return request(ctx, requestConn, chatID, userID)
		}))
		require.Eventually(t, func() bool {
			var blocked bool
			err := sqlDB.QueryRowContext(ctx, `SELECT $2::int = ANY(pg_blocking_pids($1))`, requestPID, migrationPID).Scan(&blocked)
			return err == nil && blocked
		}, testutil.WaitMedium, testutil.IntervalFast, "the chat request never waited for the upgrade")
	}

	targetSQL := readUpMigration(t, target)
	migrated := async(func() error {
		_, err := tx.ExecContext(ctx, targetSQL)
		return err
	})
	require.NoError(t, testutil.TryReceive(ctx, t, migrated), "migration %d", target)
	require.NoError(t, tx.Commit())

	for _, result := range requestResults {
		err := testutil.TryReceive(ctx, t, result)
		pqErr, ok := errors.AsType[*pq.Error](err)
		require.Truef(t, ok && pqErr.Code == "40P01", "chat request: want deadlock_detected, got %v", err)
	}
}

// readUpMigration returns the SQL of the up migration with the given version
// from the migrations directory.
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
