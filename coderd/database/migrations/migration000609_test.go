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
// relations the way chatd does (see chatTrafficCases), and checks that
// neither side deadlocks.
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

	chatID := insertChatFixture(t, sqlDB)

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

	for _, tc := range chatTrafficCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t, chatTrafficEnv{db: sqlDB, chatID: chatID, startMigration: startMigration})
		})
	}
}

// insertChatFixture inserts an organization, a user, a model config and a
// chat, using only columns that exist since before migration 000585, and
// returns the chat ID.
func insertChatFixture(t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()
	ctx := testutil.Context(t, testutil.WaitLong)
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
		_, err := db.ExecContext(ctx, q.sql, q.args...)
		require.NoError(t, err)
	}
	return chatID
}

// chatTrafficEnv is what a chatTrafficCase needs: a database with a chat
// fixture and a way to start the migration under test.
type chatTrafficEnv struct {
	// db opens the application connections.
	db     *sql.DB
	chatID uuid.UUID
	// startMigration starts the migration and returns the PID of the
	// backend that runs it and a channel that receives its result.
	startMigration func(t *testing.T, ctx context.Context) (pid int, done <-chan error)
}

// chatTrafficCases use the chat tables the way chatd does while a migration
// runs. Each case blocks the migration on an open application transaction,
// waits until pg_blocking_pids shows the migration waiting on it, then lets
// the application continue. Neither side may get a deadlock error.
var chatTrafficCases = []struct {
	name string
	run  func(t *testing.T, env chatTrafficEnv)
}{
	{
		// chatd locks a chat row, then updates the chat. The migration must
		// not hold a lock on chats that blocks the update while it waits for
		// the row lock to go away.
		name: "LockChatThenUpdate",
		run: func(t *testing.T, env chatTrafficEnv) {
			ctx := testutil.Context(t, testutil.WaitLong)
			tx, appPID := lockChat(ctx, t, env)

			pid, migrated := env.startMigration(t, ctx)
			requireBlockedBy(ctx, t, env.db, pid, appPID)

			requireNoDeadlock(ctx, t, "application", async(updateChatAndCommit(ctx, tx, env.chatID)))
			requireNoDeadlock(ctx, t, "migration", migrated)
		},
	},
	{
		// chatd inserts a message, whose triggers read and update chats,
		// then updates the chat.
		name: "InsertMessageThenUpdateChat",
		run: func(t *testing.T, env chatTrafficEnv) {
			ctx := testutil.Context(t, testutil.WaitLong)
			conn, appPID := dedicatedConn(ctx, t, env.db)
			tx, err := conn.BeginTx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			_, err = tx.ExecContext(ctx, `
				INSERT INTO chat_messages (chat_id, role, content_version)
				VALUES ($1, 'user', 1)`, env.chatID)
			require.NoError(t, err)

			pid, migrated := env.startMigration(t, ctx)
			requireBlockedBy(ctx, t, env.db, pid, appPID)

			requireNoDeadlock(ctx, t, "application", async(updateChatAndCommit(ctx, tx, env.chatID)))
			requireNoDeadlock(ctx, t, "migration", migrated)
		},
	},
	{
		// GetChatByID reads chats_expanded, which locks the view before
		// chats. A reader that arrives while the migration waits for chats
		// must not end up holding the view lock the migration needs later.
		name: "ReadViewWhileMigrationWaits",
		run: func(t *testing.T, env chatTrafficEnv) {
			ctx := testutil.Context(t, testutil.WaitLong)
			tx, appPID := lockChat(ctx, t, env)

			pid, migrated := env.startMigration(t, ctx)
			requireBlockedBy(ctx, t, env.db, pid, appPID)

			reader, readerPID := dedicatedConn(ctx, t, env.db)
			read := async(func() error {
				var id uuid.UUID
				return reader.QueryRowContext(ctx, `SELECT id FROM chats_expanded WHERE id = $1`, env.chatID).Scan(&id)
			})
			// The migration retries its locks, so the reader may finish
			// without ever waiting. Either way, it must have started before
			// the application commits.
			require.Eventually(t, func() bool {
				return len(read) > 0 || isBlockedBy(ctx, env.db, readerPID, pid)
			}, testutil.WaitMedium, testutil.IntervalFast, "reader neither finished nor waited for the migration")

			require.NoError(t, tx.Commit())
			requireNoDeadlock(ctx, t, "reader", read)
			requireNoDeadlock(ctx, t, "migration", migrated)
		},
	},
	{
		// A transaction that already locked a chat reads chats_expanded, as
		// dbauthz does when it fetches a chat it is about to update.
		name: "LockChatThenReadView",
		run: func(t *testing.T, env chatTrafficEnv) {
			ctx := testutil.Context(t, testutil.WaitLong)
			tx, appPID := lockChat(ctx, t, env)

			pid, migrated := env.startMigration(t, ctx)
			requireBlockedBy(ctx, t, env.db, pid, appPID)

			read := async(func() error {
				var id uuid.UUID
				if err := tx.QueryRowContext(ctx, `SELECT id FROM chats_expanded WHERE id = $1`, env.chatID).Scan(&id); err != nil {
					return err
				}
				return tx.Commit()
			})
			requireNoDeadlock(ctx, t, "application", read)
			requireNoDeadlock(ctx, t, "migration", migrated)
		},
	},
}

// lockChat opens an application transaction that holds a row lock on the
// chat, as GetChatByIDForUpdate does, and returns it with its backend PID.
func lockChat(ctx context.Context, t *testing.T, env chatTrafficEnv) (*sql.Tx, int) {
	t.Helper()
	conn, pid := dedicatedConn(ctx, t, env.db)
	tx, err := conn.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.ExecContext(ctx, `SELECT id FROM chats WHERE id = $1 FOR UPDATE`, env.chatID)
	require.NoError(t, err)
	return tx, pid
}

func updateChatAndCommit(ctx context.Context, tx *sql.Tx, chatID uuid.UUID) func() error {
	return func() error {
		_, err := tx.ExecContext(ctx, `UPDATE chats SET title = 'renamed' WHERE id = $1`, chatID)
		if err != nil {
			return err
		}
		return tx.Commit()
	}
}

// isBlockedBy reports whether pid waits for a lock held or requested by
// blocker.
func isBlockedBy(ctx context.Context, db *sql.DB, pid, blocker int) bool {
	var blocked bool
	err := db.QueryRowContext(ctx, `SELECT $2::int = ANY(pg_blocking_pids($1))`, pid, blocker).Scan(&blocked)
	return err == nil && blocked
}

func requireBlockedBy(ctx context.Context, t *testing.T, db *sql.DB, pid, blocker int) {
	t.Helper()
	require.Eventually(t, func() bool { return isBlockedBy(ctx, db, pid, blocker) },
		testutil.WaitMedium, testutil.IntervalFast, "pid %d never waited for pid %d", pid, blocker)
}

// requireNoDeadlock waits for the result on done and fails the test if it
// is an error, with the deadlock details for a deadlock error.
func requireNoDeadlock(ctx context.Context, t *testing.T, who string, done <-chan error) {
	t.Helper()
	err := testutil.TryReceive(ctx, t, done)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "40P01" {
		t.Fatalf("%s: %v: %s", who, err, pqErr.Detail)
	}
	require.NoError(t, err, who)
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
