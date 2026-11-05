package migrations_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// migrationFS returns one migration per statement, named step_1, step_2, ...
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

func field(t *testing.T, e slog.SinkEntry, name string) any {
	t.Helper()
	for _, f := range e.Fields {
		if f.Name == name {
			return f.Value
		}
	}
	t.Fatalf("line %q has no field %q: %v", e.Message, name, e.Fields)
	return nil
}

func fieldNames(e slog.SinkEntry) []string {
	names := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		names = append(names, f.Name)
	}
	return names
}

func onlyEntry(t *testing.T, sink *testutil.FakeSink, msg string) slog.SinkEntry {
	t.Helper()
	entries := sink.Entries(hasMessage(msg))
	require.Len(t, entries, 1, "lines with message %q", msg)
	return entries[0]
}

// waitForEntry waits until a line with msg has been logged and returns it.
func waitForEntry(ctx context.Context, t *testing.T, sink *testutil.FakeSink, msg string) slog.SinkEntry {
	t.Helper()
	testutil.Eventually(ctx, t, func(context.Context) bool {
		return len(sink.Entries(hasMessage(msg))) > 0
	}, testutil.IntervalFast)
	return sink.Entries(hasMessage(msg))[0]
}

// waitForLockWait waits until the migration connection, identified by its
// application name in the test's database, is waiting on a lock.
func waitForLockWait(ctx context.Context, t *testing.T, db *sql.DB, appName string) {
	t.Helper()
	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		var n int
		err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity
WHERE application_name = $1 AND datname = current_database() AND wait_event_type = 'Lock'`,
			appName).Scan(&n)
		return err == nil && n > 0
	}, testutil.IntervalFast)
}

// blockingObserver is a database whose queries block until released and
// ignore cancellation, like a PostgreSQL driver in the middle of a query.
type blockingObserver struct {
	entered      chan struct{}
	release      chan struct{}
	returned     chan struct{}
	enteredOnce  sync.Once
	returnedOnce sync.Once
}

func newBlockingObserver(t *testing.T) (*blockingObserver, *sql.DB) {
	t.Helper()
	o := &blockingObserver{
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
		returned: make(chan struct{}),
	}
	db := sql.OpenDB(o)
	t.Cleanup(func() { _ = db.Close() })
	return o, db
}

func (o *blockingObserver) Connect(context.Context) (driver.Conn, error) {
	return blockingConn{o: o}, nil
}

func (o *blockingObserver) Driver() driver.Driver { return blockingDriver{o: o} }

type blockingDriver struct{ o *blockingObserver }

func (d blockingDriver) Open(string) (driver.Conn, error) { return blockingConn(d), nil }

type blockingConn struct{ o *blockingObserver }

func (blockingConn) Prepare(string) (driver.Stmt, error) { return nil, xerrors.New("not supported") }
func (blockingConn) Close() error                        { return nil }
func (blockingConn) Begin() (driver.Tx, error)           { return nil, xerrors.New("not supported") }

func (c blockingConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	c.o.enteredOnce.Do(func() { close(c.o.entered) })
	<-c.o.release
	c.o.returnedOnce.Do(func() { close(c.o.returned) })
	return nil, xerrors.New("observer released")
}

func backendPID(ctx context.Context, t *testing.T, tx *sql.Tx) int {
	t.Helper()
	var pid int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
	return pid
}

func TestReporter(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		db := testSQLDB(t)
		require.NoError(t, migrations.Up(db, migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`))))

		sink := testutil.NewFakeSink(t)
		err := migrations.Up(db, migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`, `CREATE TABLE b (id int);`, `CREATE TABLE c (id int);`)),
			migrations.WithLogger(sink.Logger()),
		)
		require.NoError(t, err)

		required := onlyEntry(t, sink, "database migration required")
		require.Equal(t, slog.LevelInfo, required.Level)
		require.Equal(t, []string{"from_version", "target_version", "pending"}, fieldNames(required))
		require.Equal(t, 1, field(t, required, "from_version"))
		require.Equal(t, 3, field(t, required, "target_version"))
		require.Equal(t, 2, field(t, required, "pending"))

		for position, version := range []int{2, 3} {
			position++
			applying := onlyEntry(t, sink, fmt.Sprintf("applying update %d of 2", position))
			require.Equal(t, slog.LevelInfo, applying.Level)
			require.Equal(t, []string{"version", "name", "position", "total"}, fieldNames(applying))
			require.Equal(t, version, field(t, applying, "version"))
			require.Equal(t, fmt.Sprintf("step_%d", version), field(t, applying, "name"))
			require.Equal(t, position, field(t, applying, "position"))
			require.Equal(t, 2, field(t, applying, "total"))
		}

		applied := sink.Entries(hasMessage("database migration applied, pending commit"))
		require.Len(t, applied, 2)
		for i, e := range applied {
			require.Equal(t, slog.LevelInfo, e.Level)
			require.Equal(t, []string{"version", "name", "position", "total", "duration_ms"}, fieldNames(e))
			require.Equal(t, i+2, field(t, e, "version"))
			require.Equal(t, i+1, field(t, e, "position"))
			require.IsType(t, int64(0), field(t, e, "duration_ms"))
		}

		// The lock was free, so nothing about waiting for it is logged.
		require.Empty(t, sink.Entries(hasMessage("waiting for database migration lock")))
		require.Empty(t, sink.Entries(hasMessage("acquired database migration lock")))
	})

	t.Run("UpToDate", func(t *testing.T) {
		t.Parallel()
		db := testSQLDB(t)
		fsys := migrationFS(`CREATE TABLE a (id int);`)
		require.NoError(t, migrations.Up(db, migrations.WithFS(fsys)))

		sink := testutil.NewFakeSink(t)
		require.NoError(t, migrations.Up(db, migrations.WithFS(fsys),
			migrations.WithLogger(sink.Logger()),
		))

		entries := sink.Entries()
		require.Len(t, entries, 1)
		require.Equal(t, "database schema is up to date", entries[0].Message)
		require.Equal(t, slog.LevelInfo, entries[0].Level)
		require.Equal(t, 1, field(t, entries[0], "version"))
	})

	t.Run("FreshInstall", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)
		const appName = "test/fresh-install"
		// An empty database has no tables to lock, so the second step waits
		// on an advisory lock held by the test instead.
		const stepLock = 4242

		holder, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = holder.Rollback() }()
		_, err = holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, stepLock)
		require.NoError(t, err)

		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker("migrations", "heartbeat")
		defer trap.Close()
		sink := testutil.NewFakeSink(t)

		done := make(chan error, 1)
		go func() {
			done <- migrations.Up(db, migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`, fmt.Sprintf(`SELECT pg_advisory_xact_lock(%d);`, stepLock))),
				migrations.WithLogger(sink.Logger()), migrations.WithAppName(appName), migrations.WithClock(mClock), migrations.WithHeartbeatInterval(time.Second),
			)
		}()

		// One heartbeat ticker per step.
		trap.MustWait(ctx).MustRelease(ctx)
		trap.MustWait(ctx).MustRelease(ctx)

		waitForLockWait(ctx, t, db, appName)
		mClock.Advance(time.Second).MustWait(ctx)
		// Per-step lines are at debug on a fresh install, but the heartbeat
		// stays at info: it is the liveness signal of a slow step.
		beat := waitForEntry(ctx, t, sink, "still applying update 2 of 2")
		require.Equal(t, slog.LevelInfo, beat.Level)

		require.NoError(t, holder.Rollback())
		require.NoError(t, testutil.TryReceive(ctx, t, done))

		required := onlyEntry(t, sink, "database migration required")
		require.Equal(t, slog.LevelInfo, required.Level)
		// There is no version yet, so from_version is omitted, not -1.
		require.Equal(t, []string{"target_version", "pending"}, fieldNames(required))
		require.Equal(t, 2, field(t, required, "pending"))

		for _, msg := range []string{"applying update 1 of 2", "applying update 2 of 2"} {
			require.Equal(t, slog.LevelDebug, onlyEntry(t, sink, msg).Level, msg)
		}
		applied := sink.Entries(hasMessage("database migration applied, pending commit"))
		require.Len(t, applied, 2)
		for _, e := range applied {
			require.Equal(t, slog.LevelDebug, e.Level)
		}
	})

	t.Run("WaitsForMigrationLock", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)
		const appName = "test/waits-for-migration-lock"

		holder, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = holder.Rollback() }()
		holderPID := backendPID(ctx, t, holder)
		_, err = holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, migrations.LockID)
		require.NoError(t, err)

		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker("migrations", "heartbeat")
		sink := testutil.NewFakeSink(t)

		done := make(chan error, 1)
		go func() {
			done <- migrations.Up(db, migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`)),
				migrations.WithLogger(sink.Logger()), migrations.WithAppName(appName), migrations.WithClock(mClock), migrations.WithHeartbeatInterval(time.Second),
			)
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		// The step's heartbeat after the wait must not block on the trap.
		trap.Close()

		waiting := onlyEntry(t, sink, "waiting for database migration lock")
		require.Equal(t, slog.LevelInfo, waiting.Level)
		require.Equal(t, []string{"phase"}, fieldNames(waiting))
		require.Equal(t, "setup", field(t, waiting, "phase"))

		waitForLockWait(ctx, t, db, appName)
		mClock.Advance(time.Second).MustWait(ctx)
		beat := waitForEntry(ctx, t, sink, "still waiting for database migration lock")
		require.Equal(t, slog.LevelInfo, beat.Level)
		require.Equal(t, []string{"elapsed_ms", "state", "wait_event", "blocked_by"}, fieldNames(beat))
		require.Equal(t, int64(1000), field(t, beat, "elapsed_ms"))
		require.Equal(t, "waiting_on_lock", field(t, beat, "state"))
		require.Equal(t, "advisory", field(t, beat, "wait_event"))
		require.Equal(t, strconv.Itoa(holderPID), field(t, beat, "blocked_by"))

		require.NoError(t, holder.Rollback())
		require.NoError(t, testutil.TryReceive(ctx, t, done))

		acquired := onlyEntry(t, sink, "acquired database migration lock")
		require.Equal(t, []string{"phase", "waited_ms"}, fieldNames(acquired))
		require.Equal(t, "setup", field(t, acquired, "phase"))
		require.Equal(t, int64(1000), field(t, acquired, "waited_ms"))
		// The lock was free for the batch, so only one wait is logged.
		require.Len(t, sink.Entries(hasMessage("waiting for database migration lock")), 1)
	})

	t.Run("StepBlockedByTableLock", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)
		const appName = "test/step-blocked-by-table-lock"
		fsys := migrationFS(`CREATE TABLE a (id int);`, `CREATE TABLE b (id int);`, `ALTER TABLE a ADD COLUMN c int;`)
		require.NoError(t, migrations.Up(db, migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`))))

		blocker, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = blocker.Rollback() }()
		blockerPID := backendPID(ctx, t, blocker)
		_, err = blocker.ExecContext(ctx, `LOCK TABLE a IN ACCESS SHARE MODE`)
		require.NoError(t, err)

		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker("migrations", "heartbeat")
		defer trap.Close()
		sink := testutil.NewFakeSink(t)

		done := make(chan error, 1)
		go func() {
			done <- migrations.Up(db, migrations.WithFS(fsys),
				migrations.WithLogger(sink.Logger()), migrations.WithAppName(appName), migrations.WithClock(mClock), migrations.WithHeartbeatInterval(time.Second),
			)
		}()

		// One heartbeat ticker per step.
		trap.MustWait(ctx).MustRelease(ctx)
		trap.MustWait(ctx).MustRelease(ctx)

		waitForLockWait(ctx, t, db, appName)
		mClock.Advance(time.Second).MustWait(ctx)
		beat := waitForEntry(ctx, t, sink, "still applying update 2 of 2")
		require.Equal(t, slog.LevelInfo, beat.Level)
		require.Equal(t, []string{
			"elapsed_ms", "state", "wait_event", "blocked_by", "version", "name", "position", "total",
		}, fieldNames(beat))
		require.Equal(t, int64(1000), field(t, beat, "elapsed_ms"))
		require.Equal(t, "waiting_on_lock", field(t, beat, "state"))
		require.Equal(t, "relation", field(t, beat, "wait_event"))
		require.Equal(t, strconv.Itoa(blockerPID), field(t, beat, "blocked_by"))
		require.Equal(t, 3, field(t, beat, "version"))
		require.Equal(t, "step_3", field(t, beat, "name"))
		require.Equal(t, 2, field(t, beat, "position"))
		require.Equal(t, 2, field(t, beat, "total"))

		require.NoError(t, blocker.Rollback())
		require.NoError(t, testutil.TryReceive(ctx, t, done))
		applied := sink.Entries(hasMessage("database migration applied, pending commit"))
		require.Len(t, applied, 2)
		require.Equal(t, int64(1000), field(t, applied[1], "duration_ms"))
	})

	t.Run("HeartbeatCheckFails", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)
		const appName = "test/heartbeat-check-fails"

		// A closed pool fails every reporting query.
		observer, err := sql.Open("postgres", "")
		require.NoError(t, err)
		require.NoError(t, observer.Close())

		holder, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = holder.Rollback() }()
		_, err = holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, migrations.LockID)
		require.NoError(t, err)

		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker("migrations", "heartbeat")
		sink := testutil.NewFakeSink(t)

		done := make(chan error, 1)
		go func() {
			done <- migrations.Up(db, migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`)),
				migrations.WithLogger(sink.Logger()), migrations.WithAppName(appName), migrations.WithClock(mClock), migrations.WithHeartbeatInterval(time.Second), migrations.WithObserverDB(observer),
			)
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		trap.Close()

		waitForLockWait(ctx, t, db, appName)
		for i := 1; i <= 2; i++ {
			mClock.Advance(time.Second).MustWait(ctx)
			testutil.Eventually(ctx, t, func(context.Context) bool {
				return len(sink.Entries(hasMessage("still waiting for database migration lock"))) >= i
			}, testutil.IntervalFast)
		}
		for _, beat := range sink.Entries(hasMessage("still waiting for database migration lock")) {
			require.Equal(t, []string{"elapsed_ms", "state"}, fieldNames(beat))
			require.Equal(t, "unknown", field(t, beat, "state"))
		}
		// Only the first failed check is reported.
		warn := onlyEntry(t, sink, "database migration progress details unavailable")
		require.Equal(t, slog.LevelWarn, warn.Level)

		require.NoError(t, holder.Rollback())
		require.NoError(t, testutil.TryReceive(ctx, t, done))
		var version int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT version FROM schema_migrations`).Scan(&version))
		require.Equal(t, 1, version)
	})

	t.Run("LateHeartbeatDiscarded", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)
		const appName = "test/late-heartbeat-discarded"
		fsys := migrationFS(`CREATE TABLE a (id int);`, `ALTER TABLE a ADD COLUMN c int;`)
		require.NoError(t, migrations.Up(db, migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`))))

		blocker, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = blocker.Rollback() }()
		_, err = blocker.ExecContext(ctx, `LOCK TABLE a IN ACCESS SHARE MODE`)
		require.NoError(t, err)

		observer, observerDB := newBlockingObserver(t)
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker("migrations", "heartbeat")
		defer trap.Close()
		sink := testutil.NewFakeSink(t)

		done := make(chan error, 1)
		go func() {
			done <- migrations.Up(db, migrations.WithFS(fsys),
				migrations.WithLogger(sink.Logger()), migrations.WithAppName(appName), migrations.WithClock(mClock), migrations.WithHeartbeatInterval(time.Second), migrations.WithObserverDB(observerDB),
			)
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		waitForLockWait(ctx, t, db, appName)
		mClock.Advance(time.Second).MustWait(ctx)
		// The heartbeat check is in flight and ignores cancellation.
		_ = testutil.TryReceive(ctx, t, observer.entered)

		// The step finishes while the check is still blocked: stopping the
		// heartbeat must not wait for it.
		require.NoError(t, blocker.Rollback())
		require.NoError(t, testutil.TryReceive(ctx, t, done))
		require.Len(t, sink.Entries(hasMessage("database migration applied, pending commit")), 1)

		// Once the late check returns, its result is discarded. This can only
		// pass falsely, if the discarded line were still being written.
		close(observer.release)
		_ = testutil.TryReceive(ctx, t, observer.returned)
		require.Empty(t, sink.Entries(hasMessage("still applying update 1 of 1")))
		require.Empty(t, sink.Entries(hasMessage("database migration progress details unavailable")))
	})
}
