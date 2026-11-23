package migrations_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

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

// requireGolden compares the sink's output with testdata/reporter/name.golden.
func requireGolden(t *testing.T, sink *migrations.GoldenSink, name string) {
	t.Helper()
	migrations.RequireGolden(t, name, sink.String())
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
	return blockingObserverConn{o: o}, nil
}

func (o *blockingObserver) Driver() driver.Driver { return blockingObserverDriver{o: o} }

type blockingObserverDriver struct{ o *blockingObserver }

func (d blockingObserverDriver) Open(string) (driver.Conn, error) {
	return blockingObserverConn(d), nil
}

type blockingObserverConn struct{ o *blockingObserver }

func (blockingObserverConn) Prepare(string) (driver.Stmt, error) {
	return nil, xerrors.New("not supported")
}
func (blockingObserverConn) Close() error              { return nil }
func (blockingObserverConn) Begin() (driver.Tx, error) { return nil, xerrors.New("not supported") }

func (c blockingObserverConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
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

// testOptions are the options every test run uses: the sink's logger, a mock
// clock so durations are stable, and a one second heartbeat.
func testOptions(sink *migrations.GoldenSink, mClock *quartz.Mock, appName string, fsys fstest.MapFS) []migrations.Option {
	return []migrations.Option{
		migrations.WithFS(fsys),
		migrations.WithLogger(sink.Logger()),
		migrations.WithAppName(appName),
		migrations.WithClock(mClock),
		migrations.WithHeartbeatInterval(time.Second),
	}
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

		sink := migrations.NewGoldenSink()
		fsys := migrationFS(`CREATE TABLE a (id int);`, `CREATE TABLE b (id int);`, `CREATE TABLE c (id int);`)
		require.NoError(t, migrations.Up(db, testOptions(sink, quartz.NewMock(t), "test/success", fsys)...))

		requireGolden(t, sink, "success")
	})

	t.Run("UpToDate", func(t *testing.T) {
		t.Parallel()
		db := testSQLDB(t)
		fsys := migrationFS(`CREATE TABLE a (id int);`)
		require.NoError(t, migrations.Up(db, migrations.WithFS(fsys)))

		sink := migrations.NewGoldenSink()
		require.NoError(t, migrations.Up(db, testOptions(sink, quartz.NewMock(t), "test/up-to-date", fsys)...))

		requireGolden(t, sink, "up_to_date")
	})

	t.Run("WaitsForMigrationLock", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)
		const appName = "test/waits-for-migration-lock"

		holder, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = holder.Rollback() }()
		sink := migrations.NewGoldenSink()
		sink.NamePID(backendPID(ctx, t, holder), "holder")
		_, err = holder.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, migrations.LockID)
		require.NoError(t, err)

		mClock := quartz.NewMock(t)
		trap := mClock.Trap().TickerFunc("migrations", "heartbeat")

		done := make(chan error, 1)
		go func() {
			done <- migrations.Up(db, testOptions(sink, mClock, appName, migrationFS(`CREATE TABLE a (id int);`))...)
		}()

		// The setup lock's heartbeat; later heartbeats must not block on the
		// trap.
		trap.MustWait(ctx).MustRelease(ctx)
		trap.Close()
		waitForLockWait(ctx, t, db, appName)
		mClock.Advance(time.Second).MustWait(ctx)

		require.NoError(t, holder.Rollback())
		require.NoError(t, testutil.RequireReceive(ctx, t, done))

		requireGolden(t, sink, "waits_for_migration_lock")
	})

	t.Run("StepBlockedByTableLock", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)
		const appName = "test/step-blocked-by-table-lock"
		require.NoError(t, migrations.Up(db, migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`))))

		blocker, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = blocker.Rollback() }()
		sink := migrations.NewGoldenSink()
		sink.NamePID(backendPID(ctx, t, blocker), "blocker")
		_, err = blocker.ExecContext(ctx, `LOCK TABLE a IN ACCESS SHARE MODE`)
		require.NoError(t, err)

		mClock := quartz.NewMock(t)
		trap := mClock.Trap().TickerFunc("migrations", "heartbeat")
		defer trap.Close()

		fsys := migrationFS(`CREATE TABLE a (id int);`, `CREATE TABLE b (id int);`, `ALTER TABLE a ADD COLUMN c int;`)
		done := make(chan error, 1)
		go func() {
			done <- migrations.Up(db, testOptions(sink, mClock, appName, fsys)...)
		}()

		// One heartbeat per lock (setup, batch) and per step.
		for range 4 {
			trap.MustWait(ctx).MustRelease(ctx)
		}
		waitForLockWait(ctx, t, db, appName)
		mClock.Advance(time.Second).MustWait(ctx)

		require.NoError(t, blocker.Rollback())
		require.NoError(t, testutil.RequireReceive(ctx, t, done))

		requireGolden(t, sink, "step_blocked_by_table_lock")
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
		trap := mClock.Trap().TickerFunc("migrations", "heartbeat")
		sink := migrations.NewGoldenSink()

		opts := append(testOptions(sink, mClock, appName, migrationFS(`CREATE TABLE a (id int);`)), migrations.WithObserverDB(observer))
		done := make(chan error, 1)
		go func() {
			done <- migrations.Up(db, opts...)
		}()

		trap.MustWait(ctx).MustRelease(ctx)
		trap.Close()
		waitForLockWait(ctx, t, db, appName)
		// Two failed checks: both heartbeats are written, the warning only
		// once.
		mClock.Advance(time.Second).MustWait(ctx)
		mClock.Advance(time.Second).MustWait(ctx)

		require.NoError(t, holder.Rollback())
		require.NoError(t, testutil.RequireReceive(ctx, t, done))

		requireGolden(t, sink, "heartbeat_check_fails")
		// The failed checks did not affect the migration.
		var version int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT version FROM schema_migrations`).Scan(&version))
		require.Equal(t, 1, version)
	})

	t.Run("LateHeartbeatDiscarded", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		db := testSQLDB(t)
		const appName = "test/late-heartbeat-discarded"
		require.NoError(t, migrations.Up(db, migrations.WithFS(migrationFS(`CREATE TABLE a (id int);`))))

		blocker, err := db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = blocker.Rollback() }()
		_, err = blocker.ExecContext(ctx, `LOCK TABLE a IN ACCESS SHARE MODE`)
		require.NoError(t, err)

		observer, observerDB := newBlockingObserver(t)
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().TickerFunc("migrations", "heartbeat")
		defer trap.Close()
		sink := migrations.NewGoldenSink()

		fsys := migrationFS(`CREATE TABLE a (id int);`, `ALTER TABLE a ADD COLUMN c int;`)
		opts := append(testOptions(sink, mClock, appName, fsys), migrations.WithObserverDB(observerDB))
		done := make(chan error, 1)
		go func() {
			done <- migrations.Up(db, opts...)
		}()

		// One heartbeat per lock (setup, batch) and the step.
		for range 3 {
			trap.MustWait(ctx).MustRelease(ctx)
		}
		waitForLockWait(ctx, t, db, appName)
		// Advance waits for the tick to finish, and this tick blocks until the
		// observer is released, so wait for it only at the end.
		advance := mClock.Advance(time.Second)
		// The heartbeat check is in flight and ignores cancellation.
		_ = testutil.TryReceive(ctx, t, observer.entered)

		// The step finishes and Up returns while the check is still blocked:
		// stopping the heartbeat does not wait for it.
		require.NoError(t, blocker.Rollback())
		require.NoError(t, testutil.RequireReceive(ctx, t, done))

		// Once the late check returns, its result is discarded, so the golden
		// file has no heartbeat or warning line.
		close(observer.release)
		_ = testutil.TryReceive(ctx, t, observer.returned)
		advance.MustWait(ctx)
		requireGolden(t, sink, "late_heartbeat_discarded")
	})
}
