package migrations_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// migrationLockID mirrors lockID in txnmigrator.go.
const migrationLockID = int64(1037453835920848937)

func migrationFS(stmts ...string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for i, stmt := range stmts {
		name := fmt.Sprintf("%06d_step_%d.up.sql", i+1, i+1)
		fsys[name] = &fstest.MapFile{Data: []byte(stmt)}
	}
	return fsys
}

func hasMessage(msg string) func(slog.SinkEntry) bool {
	return func(e slog.SinkEntry) bool { return e.Message == msg }
}

func field(e slog.SinkEntry, name string) any {
	for _, f := range e.Fields {
		if f.Name == name {
			return f.Value
		}
	}
	return nil
}

func schemaVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	require.NoError(t, db.QueryRow(`SELECT version FROM schema_migrations`).Scan(&v))
	return v
}

func TestUpWithReporter(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	t.Run("Committed", func(t *testing.T) {
		t.Parallel()
		db := testSQLDB(t)
		require.NoError(t, migrations.UpWithFS(db, migrationFS(`CREATE TABLE a (id int);`)))

		sink := testutil.NewFakeSink(t)
		err := migrations.UpWithReporter(db, migrations.ReporterOptions{
			Logger: sink.Logger(),
			FS:     migrationFS(`CREATE TABLE a (id int);`, `CREATE TABLE b (id int);`, `CREATE TABLE c (id int);`),
		})
		require.NoError(t, err)

		required := sink.Entries(hasMessage("database migration required"))
		require.Len(t, required, 1)
		require.Equal(t, "1", field(required[0], "from_version"))
		require.Equal(t, 3, field(required[0], "target_version"))
		require.Equal(t, 2, field(required[0], "pending"))

		applying := sink.Entries(hasMessage("database migration applying migration"))
		require.Len(t, applying, 2)
		require.Equal(t, "000002_step_2", field(applying[0], "migration"))
		require.Equal(t, "1/2", field(applying[0], "position"))
		require.Equal(t, "2/2", field(applying[1], "position"))
		require.Equal(t, slog.LevelInfo, applying[0].Level)

		committed := sink.Entries(hasMessage("database migration committed"))
		require.Len(t, committed, 1)
		require.Equal(t, "committed", field(committed[0], "transaction_status"))
		require.Equal(t, 3, field(committed[0], "schema_version"))
		require.Equal(t, 3, schemaVersion(t, db))
	})

	t.Run("RolledBack", func(t *testing.T) {
		t.Parallel()
		db := testSQLDB(t)
		require.NoError(t, migrations.UpWithFS(db, migrationFS(`CREATE TABLE a (id int);`)))

		sink := testutil.NewFakeSink(t)
		err := migrations.UpWithReporter(db, migrations.ReporterOptions{
			Logger: sink.Logger(),
			FS:     migrationFS(`CREATE TABLE a (id int);`, `CREATE TABLE b (id int);`, `SELECT 1/0;`),
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "migration 000003_step_3 (2/2) failed: division by zero (sqlstate=22012); rolled back, schema unchanged at 1")
		require.NotContains(t, err.Error(), "SELECT 1/0", "error must not include the migration SQL")

		failed := sink.Entries(hasMessage("database migration failed"))
		require.Len(t, failed, 1)
		require.Equal(t, "22012", field(failed[0], "sqlstate"))

		rolledBack := sink.Entries(hasMessage("database migration rolled back"))
		require.Len(t, rolledBack, 1)
		require.Equal(t, "aborted", field(rolledBack[0], "transaction_status"))

		require.Equal(t, 1, schemaVersion(t, db))
		var exists bool
		require.NoError(t, db.QueryRow(`SELECT to_regclass('b') IS NOT NULL`).Scan(&exists))
		require.False(t, exists, "migration 2 must have been rolled back")
	})

	t.Run("UpToDate", func(t *testing.T) {
		t.Parallel()
		db := testSQLDB(t)
		fsys := migrationFS(`CREATE TABLE a (id int);`)
		require.NoError(t, migrations.UpWithFS(db, fsys))

		sink := testutil.NewFakeSink(t)
		require.NoError(t, migrations.UpWithReporter(db, migrations.ReporterOptions{Logger: sink.Logger(), FS: fsys}))
		require.Len(t, sink.Entries(hasMessage("database schema is up to date")), 1)
		require.Empty(t, sink.Entries(hasMessage("database migration committed")))
	})

	t.Run("WaitsForMigrationLock", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)
		fsys := migrationFS(`CREATE TABLE a (id int);`)

		holder, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = holder.Rollback() }()
		var holderPID int
		require.NoError(t, holder.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&holderPID))
		_, err = holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, migrationLockID)
		require.NoError(t, err)

		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker("migrations", "heartbeat")
		sink := testutil.NewFakeSink(t)

		done := make(chan error, 1)
		go func() {
			done <- migrations.UpWithReporter(db, migrations.ReporterOptions{
				Logger: sink.Logger(), FS: fsys, Clock: mClock, HeartbeatInterval: time.Second,
			})
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		// Later heartbeats (for example when the migration starts) must not
		// block on the trap.
		trap.Close()
		require.Len(t, sink.Entries(hasMessage("database migration waiting for migration lock held by another instance")), 1)
		mClock.Advance(time.Second).MustWait(ctx)
		testutil.Eventually(ctx, t, func(context.Context) bool {
			return len(sink.Entries(hasMessage("database migration still waiting for migration lock"))) > 0
		}, testutil.IntervalFast)
		beat := sink.Entries(hasMessage("database migration still waiting for migration lock"))[0]
		require.Equal(t, "waiting_on_lock", field(beat, "state"))
		require.Equal(t, "advisory", field(beat, "wait_event"))
		require.Contains(t, field(beat, "blocked_by"), fmt.Sprintf("pid %d", holderPID))

		require.NoError(t, holder.Rollback())
		require.NoError(t, testutil.TryReceive(ctx, t, done))
		require.Len(t, sink.Entries(hasMessage("database migration lock acquired")), 1)
	})

	t.Run("MigrationBlockedByTableLock", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)
		require.NoError(t, migrations.UpWithFS(db, migrationFS(`CREATE TABLE a (id int);`)))

		blocker, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = blocker.Rollback() }()
		_, err = blocker.ExecContext(ctx, `SELECT set_config('application_name', 'old-coder-pod', true)`)
		require.NoError(t, err)
		_, err = blocker.ExecContext(ctx, `LOCK TABLE a IN ACCESS SHARE MODE`)
		require.NoError(t, err)

		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker("migrations", "heartbeat")
		sink := testutil.NewFakeSink(t)

		done := make(chan error, 1)
		go func() {
			done <- migrations.UpWithReporter(db, migrations.ReporterOptions{
				Logger: sink.Logger(), Clock: mClock, HeartbeatInterval: time.Second,
				FS: migrationFS(`CREATE TABLE a (id int);`, `ALTER TABLE a ADD COLUMN b int;`),
			})
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		// Later heartbeats (for example when the migration starts) must not
		// block on the trap.
		trap.Close()
		// The heartbeat ticker starts just before the migration statement is
		// sent; wait until the statement is actually blocked.
		testutil.Eventually(ctx, t, func(ctx context.Context) bool {
			var n int
			_ = db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND query LIKE 'ALTER TABLE a%'`).Scan(&n)
			return n > 0
		}, testutil.IntervalFast)
		mClock.Advance(time.Second).MustWait(ctx)
		testutil.Eventually(ctx, t, func(context.Context) bool {
			return len(sink.Entries(hasMessage("database migration still running"))) > 0
		}, testutil.IntervalFast)
		beat := sink.Entries(hasMessage("database migration still running"))[0]
		require.Equal(t, "000002_step_2", field(beat, "migration"))
		require.Equal(t, "waiting_on_lock", field(beat, "state"))
		require.Equal(t, "relation", field(beat, "wait_event"))
		require.Contains(t, field(beat, "blocked_by"), "app=old-coder-pod")

		require.NoError(t, blocker.Rollback())
		require.NoError(t, testutil.TryReceive(ctx, t, done))
		require.Equal(t, 2, schemaVersion(t, db))
	})
}
