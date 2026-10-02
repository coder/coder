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

	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
)

const (
	migration000612Up   = "000612_chat_automations_kind_shape.up.sql"
	migration000612Down = "000612_chat_automations_kind_shape.down.sql"
)

// migration000612Fixture holds the rows seeded at version 611. Webhook x
// targets chat b and webhook y targets chat a, so retargeting x to a makes
// its foreign key check read the chat a publisher holds.
type migration000612Fixture struct {
	chatA, chatB uuid.UUID
	x, y         uuid.UUID
	schedule     uuid.UUID
}

func seedMigration000612(t *testing.T, sqlDB *sql.DB) migration000612Fixture {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
	orgID, userID, modelID := uuid.New(), uuid.New(), uuid.New()
	f := migration000612Fixture{
		chatA: uuid.New(), chatB: uuid.New(),
		x: uuid.New(), y: uuid.New(), schedule: uuid.New(),
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO organizations (id, name, display_name, description, created_at, updated_at, default_org_member_roles)
			VALUES ($1, 'org-612', 'org-612', '', now(), now(), '{}')`, []any{orgID}},
		{`INSERT INTO users (id, email, username, hashed_password, created_at, updated_at)
			VALUES ($1, 'user612@example.com', 'user612', '\x', now(), now())`, []any{userID}},
		// Deleted, so the model needs no AI provider.
		{`INSERT INTO chat_model_configs (id, organization_id, model, context_limit, compression_threshold, deleted)
			VALUES ($1, $2, 'model-612', 1000, 50, true)`, []any{modelID, orgID}},
		{`INSERT INTO chats (id, owner_id, organization_id, last_model_config_id, title)
			VALUES ($1, $3, $4, $5, 'chat-a'), ($2, $3, $4, $5, 'chat-b')`, []any{f.chatA, f.chatB, userID, orgID, modelID}},
		{
			`INSERT INTO chat_automations (id, organization_id, owner_id, name, kind, target_mode, target_chat_id, when_busy, webhook_use, prompt)
			VALUES ($1, $3, $4, 'x', 'webhook', 'existing_chat', $5, 'queue', 'multi', 'prompt'),
			       ($2, $3, $4, 'y', 'webhook', 'existing_chat', $6, 'queue', 'multi', 'prompt')`,
			[]any{f.x, f.y, orgID, userID, f.chatB, f.chatA},
		},
		{
			`INSERT INTO chat_automations (id, organization_id, owner_id, name, kind, target_mode, new_chat_model_config_id, prompt, schedule_cron, schedule_time_zone)
			VALUES ($1, $2, $3, 'schedule', 'schedule', 'new_chat', $4, 'prompt', '0 9 * * *', 'UTC')`,
			[]any{f.schedule, orgID, userID, modelID},
		},
	} {
		_, err := sqlDB.ExecContext(ctx, q.sql, q.args...)
		require.NoError(t, err)
	}
	return f
}

// TestMigration000612ConcurrentTraffic runs the 000612 up migration while
// other connections use chat_automations the way chatd does, and checks that
// no side becomes a deadlock victim. Each case blocks the migration on an
// open application transaction, waits until pg_blocking_pids shows the
// migration waiting on it, then lets the application continue.
//
//nolint:tparallel,paralleltest // Cases share one database; each migration attempt rolls back.
func TestMigration000612ConcurrentTraffic(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 611)
	f := seedMigration000612(t, sqlDB)
	migrationSQL, err := os.ReadFile(migration000612Up)
	require.NoError(t, err)

	// A management update locks automation x, then retargets it to chat a,
	// whose foreign key check needs a key share lock on chat a. A publisher
	// holds chat a and then locks its automation y. A migration that queues
	// for the whole table between those steps closes a cycle.
	t.Run("UpdateTargetWhilePublishing", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		update, updatePID := beginLocked(ctx, t, sqlDB, `SELECT id FROM chat_automations WHERE id = $1 FOR UPDATE`, f.x)
		publish, publishPID := beginLocked(ctx, t, sqlDB, `SELECT id FROM chats WHERE id = $1 FOR UPDATE`, f.chatA)

		pid, migrated := startSQLInTx(ctx, t, sqlDB, string(migrationSQL))
		requireWaitsFor(ctx, t, sqlDB, pid, updatePID)

		published := async(execAndCommit(ctx, publish, `SELECT id FROM chat_automations WHERE id = $1 FOR UPDATE`, f.y))
		// The migration retries its lock, so the publisher may finish
		// without ever waiting. Either way, it must have started before the
		// update needs chat a.
		require.Eventually(t, func() bool {
			return len(published) > 0 || waitsFor(ctx, sqlDB, publishPID, pid)
		}, testutil.WaitMedium, testutil.IntervalFast, "publisher neither finished nor waited for the migration")
		updated := async(execAndCommit(ctx, update, `UPDATE chat_automations SET target_chat_id = $1 WHERE id = $2`, f.chatA, f.x))

		requireNoDeadlockVictim(ctx, t, "publisher", published)
		requireNoDeadlockVictim(ctx, t, "update", updated)
		requireNoDeadlockVictim(ctx, t, "migration", migrated)
	})

	// A publisher locks the chat, then the automation, then consumes the
	// webhook. The migration must not hold anything the publisher needs
	// while it waits for the publisher's row lock.
	t.Run("PublishLocksChatThenAutomation", func(t *testing.T) {
		ctx := testutil.Context(t, testutil.WaitLong)
		publish, publishPID := beginLocked(ctx, t, sqlDB, `SELECT id FROM chats WHERE id = $1 FOR UPDATE`, f.chatA)
		_, err := publish.ExecContext(ctx, `SELECT id FROM chat_automations WHERE id = $1 FOR UPDATE`, f.y)
		require.NoError(t, err)

		pid, migrated := startSQLInTx(ctx, t, sqlDB, string(migrationSQL))
		requireWaitsFor(ctx, t, sqlDB, pid, publishPID)

		requireNoDeadlockVictim(ctx, t, "publisher", async(execAndCommit(ctx, publish,
			`UPDATE chat_automations SET webhook_consumed_at = now() WHERE id = $1`, f.y)))
		requireNoDeadlockVictim(ctx, t, "migration", migrated)
	})
}

// TestMigration000612MigratorConcurrentTraffic repeats the retarget case
// against the real migrator, which runs every pending migration in one
// transaction and commits it.
func TestMigration000612MigratorConcurrentTraffic(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 611)
	f := seedMigration000612(t, sqlDB)

	ctx := testutil.Context(t, testutil.WaitLong)
	update, updatePID := beginLocked(ctx, t, sqlDB, `SELECT id FROM chat_automations WHERE id = $1 FOR UPDATE`, f.x)
	publish, publishPID := beginLocked(ctx, t, sqlDB, `SELECT id FROM chats WHERE id = $1 FOR UPDATE`, f.chatA)

	migrated := async(func() error { return migrations.Up(sqlDB) })
	pid := requireMigratorWaitsFor(ctx, t, sqlDB, updatePID)

	published := async(execAndCommit(ctx, publish, `SELECT id FROM chat_automations WHERE id = $1 FOR UPDATE`, f.y))
	require.Eventually(t, func() bool {
		return len(published) > 0 || waitsFor(ctx, sqlDB, publishPID, pid)
	}, testutil.WaitMedium, testutil.IntervalFast, "publisher neither finished nor waited for the migrator")
	updated := async(execAndCommit(ctx, update, `UPDATE chat_automations SET target_chat_id = $1 WHERE id = $2`, f.chatA, f.x))

	requireNoDeadlockVictim(ctx, t, "publisher", published)
	requireNoDeadlockVictim(ctx, t, "update", updated)
	requireNoDeadlockVictim(ctx, t, "migrator", migrated)

	var version int
	var dirty bool
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty))
	require.GreaterOrEqual(t, version, 612)
	require.False(t, dirty)
}

// TestMigration000612MigratorFrom610 runs 000611 and 000612 in the real
// migrator's single transaction while a management update holds an
// automation row and then retargets it. The update's foreign key check reads
// chats, so the migrator must not keep chats locked from 000611 while 000612
// waits for the automation row.
func TestMigration000612MigratorFrom610(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 610)
	f := seedMigration000612(t, sqlDB)

	ctx := testutil.Context(t, testutil.WaitLong)
	update, updatePID := beginLocked(ctx, t, sqlDB, `SELECT id FROM chat_automations WHERE id = $1 FOR UPDATE`, f.x)

	migrated := async(func() error { return migrations.Up(sqlDB) })
	requireMigratorWaitsFor(ctx, t, sqlDB, updatePID)

	requireNoDeadlockVictim(ctx, t, "update", async(execAndCommit(ctx, update,
		`UPDATE chat_automations SET target_chat_id = $1 WHERE id = $2`, f.chatA, f.x)))
	requireNoDeadlockVictim(ctx, t, "migrator", migrated)
}

// TestMigration000612KindShape checks that 000612 rejects each kind's
// columns on the other kind and that the down migration restores the 000609
// constraint exactly.
func TestMigration000612KindShape(t *testing.T) {
	t.Parallel()

	sqlDB := testSQLDB(t)
	stepTo(t, sqlDB, 611)
	f := seedMigration000612(t, sqlDB)
	ctx := testutil.Context(t, testutil.WaitLong)

	const defQuery = `SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conrelid = 'chat_automations'::regclass AND conname = 'chat_automations_kind_shape'`
	var before string
	require.NoError(t, sqlDB.QueryRowContext(ctx, defQuery).Scan(&before))

	stepTo(t, sqlDB, 612)
	for _, stmt := range []struct {
		name, sql string
		id        uuid.UUID
	}{
		{"WebhookWithScheduleTimeZone", `UPDATE chat_automations SET schedule_time_zone = 'UTC' WHERE id = $1`, f.x},
		{"WebhookWithScheduleNextRunAt", `UPDATE chat_automations SET schedule_next_run_at = now() WHERE id = $1`, f.x},
		{"ScheduleWithWebhookConsumedAt", `UPDATE chat_automations SET webhook_consumed_at = now() WHERE id = $1`, f.schedule},
	} {
		_, err := sqlDB.ExecContext(ctx, stmt.sql, stmt.id)
		var pqErr *pq.Error
		require.True(t, errors.As(err, &pqErr), "%s: got %v", stmt.name, err)
		require.Equal(t, pq.ErrorCode("23514"), pqErr.Code, stmt.name)
		require.Equal(t, "chat_automations_kind_shape", pqErr.Constraint, stmt.name)
	}

	downSQL, err := os.ReadFile(migration000612Down)
	require.NoError(t, err)
	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout = '5s'`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(downSQL))
	require.NoError(t, err)
	var lockTimeout, after string
	require.NoError(t, tx.QueryRowContext(ctx, `SHOW lock_timeout`).Scan(&lockTimeout))
	require.Equal(t, "5s", lockTimeout, "down migration must restore lock_timeout")
	require.NoError(t, tx.QueryRowContext(ctx, defQuery).Scan(&after))
	require.Equal(t, before, after)
}

// beginLocked opens an application transaction on its own connection and
// runs a locking statement in it. The transaction rolls back at cleanup
// unless the test commits it.
func beginLocked(ctx context.Context, t *testing.T, db *sql.DB, query string, args ...any) (*sql.Tx, int) {
	t.Helper()
	conn, pid := dedicatedConn(ctx, t, db)
	tx, err := conn.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.ExecContext(ctx, query, args...)
	require.NoError(t, err)
	return tx, pid
}

func execAndCommit(ctx context.Context, tx *sql.Tx, query string, args ...any) func() error {
	return func() error {
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return err
		}
		return tx.Commit()
	}
}

// startSQLInTx runs migrationSQL in a transaction on its own connection and
// rolls it back. It returns the backend PID and the result, sent after the
// rollback. The migration lowers lock_timeout while it takes its locks and
// must restore the caller's setting afterwards.
func startSQLInTx(ctx context.Context, t *testing.T, db *sql.DB, migrationSQL string) (int, <-chan error) {
	t.Helper()
	conn, pid := dedicatedConn(ctx, t, db)
	return pid, async(func() error {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout = '5s'`)
		if err == nil {
			_, err = tx.ExecContext(ctx, migrationSQL)
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

// waitsFor reports whether pid waits for a lock held or requested by
// blocker.
func waitsFor(ctx context.Context, db *sql.DB, pid, blocker int) bool {
	var blocked bool
	err := db.QueryRowContext(ctx, `SELECT $2::int = ANY(pg_blocking_pids($1))`, pid, blocker).Scan(&blocked)
	return err == nil && blocked
}

func requireWaitsFor(ctx context.Context, t *testing.T, db *sql.DB, pid, blocker int) {
	t.Helper()
	require.Eventually(t, func() bool { return waitsFor(ctx, db, pid, blocker) },
		testutil.WaitMedium, testutil.IntervalFast, "pid %d never waited for pid %d", pid, blocker)
}

// requireMigratorWaitsFor waits until a backend waits for updatePID and
// returns its PID. Only the migrator waits for the update transaction.
func requireMigratorWaitsFor(ctx context.Context, t *testing.T, db *sql.DB, updatePID int) int {
	t.Helper()
	var pid int
	require.Eventually(t, func() bool {
		return db.QueryRowContext(ctx, `SELECT pid FROM pg_stat_activity
			WHERE datname = current_database() AND $1::int = ANY(pg_blocking_pids(pid))`, updatePID).Scan(&pid) == nil
	}, testutil.WaitMedium, testutil.IntervalFast, "migrator never waited for the update transaction")
	return pid
}

func requireNoDeadlockVictim(ctx context.Context, t *testing.T, who string, done <-chan error) {
	t.Helper()
	err := testutil.TryReceive(ctx, t, done)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "40P01" {
		t.Fatalf("%s: %v: %s", who, err, pqErr.Detail)
	}
	require.NoError(t, err, who)
}
