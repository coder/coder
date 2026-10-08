package migrations

// Internal (white-box) scenario tests for UpWithLogger (RFC: Improve
// Coder Upgrade Visibility, requirements 1 and 2). These scenarios need
// unexported access (a custom fs.FS via runUp, for simulating failures
// without touching the real migration files), so they live in package
// migrations rather than migrations_test. They cannot import dbtestutil
// (it imports this package, which would be a cycle), so they connect
// directly to the same long-lived coder-test-postgres container
// dbtestutil itself starts and reuses across test runs.

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"

	"github.com/coder/coder/v2/testutil"
)

const scenarioAdminDSN = "postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable"

// scenarioLogger mirrors the helper of the same name in
// migrate_logging_scenarios_test.go (duplicated rather than shared, since
// the two files are in different packages).
func scenarioLogger(t *testing.T, scenario string) slog.Logger {
	t.Helper()
	dir := "test-output"
	require.NoError(t, os.MkdirAll(dir, 0o755))
	f, err := os.Create(filepath.Join(dir, scenario+".log"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	w := io.MultiWriter(f, os.Stdout)
	return slog.Make(sloghuman.Sink(w)).Leveled(slog.LevelDebug)
}

// scenarioDB creates a fresh scratch database on the shared
// coder-test-postgres container (started by dbtestutil in other tests in
// this package, and left running for reuse) and returns a connection to
// it.
func scenarioDB(t *testing.T) *sql.DB {
	t.Helper()

	admin, err := sql.Open("postgres", scenarioAdminDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })
	require.NoError(t, admin.Ping())

	name := "scenario_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	_, err = admin.Exec("CREATE DATABASE " + name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)")
	})

	dsn := fmt.Sprintf("postgres://postgres:postgres@127.0.0.1:5432/%s?sslmode=disable", name)
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.Ping())
	return db
}

// scratchMigrations copies every real migration file into a temp
// directory and returns the directory, so a scenario can append a
// throwaway broken or slow migration without ever touching the real
// coderd/database/migrations/*.sql files.
func scratchMigrations(t *testing.T) (dir string, nextVersion int) {
	t.Helper()
	dir = t.TempDir()

	entries, err := migrations.ReadDir(".")
	require.NoError(t, err)

	maxVersion := 0
	for _, e := range entries {
		content, err := migrations.ReadFile(e.Name())
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, e.Name()), content, 0o644))

		var version int
		if _, err := fmt.Sscanf(e.Name(), "%06d_", &version); err == nil && version > maxVersion {
			maxVersion = version
		}
	}
	return dir, maxVersion + 1
}

func writeScratchMigration(t *testing.T, dir string, version int, name, upSQL, downSQL string) {
	t.Helper()
	base := fmt.Sprintf("%06d_%s", version, name)
	require.NoError(t, os.WriteFile(filepath.Join(dir, base+".up.sql"), []byte(upSQL), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, base+".down.sql"), []byte(downSQL), 0o644))
}

// TestUpWithLoggerSQLFailureRollback first migrates a scratch database to
// the real, current schema (so there is a non-trivial prior version to
// revert to), then appends one valid migration followed by one broken
// migration (a duplicate CREATE TABLE), and confirms the whole batch
// rolls back: a real SQL failure, a real PostgreSQL-rolled-back
// transaction, not a simulated one. The valid migration first is
// deliberate: with only the broken migration pending, it fails before
// ever being marked applied, so len(applied) stays 0 and logOutcome
// takes the "nothing ran" path (no outcome line) instead of
// OutcomeRolledBack. A prior successful migration is what actually
// exercises the rollback classification and its WARN line.
func TestUpWithLoggerSQLFailureRollback(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db := scenarioDB(t)
	ctx := context.Background()

	// Phase 1: bring the scratch database fully up to date on the real
	// chain. Not the scenario under test, so its own log file.
	setupLogger := scenarioLogger(t, "sql-failure-rollback-setup")
	require.NoError(t, runUp(ctx, db, nil, setupLogger.Named("migrations"), true))

	var currentVersion int
	require.NoError(t, db.QueryRow("SELECT version FROM schema_migrations").Scan(&currentVersion))

	// Phase 2: the scenario. A scratch copy of the real chain plus one
	// valid migration and, after it, one broken migration.
	dir, nextVersion := scratchMigrations(t)
	require.Equal(t, currentVersion+1, nextVersion, "scratch copy should pick up right where phase 1 left off")
	writeScratchMigration(t, dir, nextVersion, "scratch_rollback",
		"CREATE TABLE scratch_rollback (id int);\n", "DROP TABLE scratch_rollback;\n")
	writeScratchMigration(t, dir, nextVersion+1, "duplicate_users_table",
		"CREATE TABLE users (id int);\n", "-- no-op\n")

	logger := scenarioLogger(t, "sql-failure-rollback")
	err := runUp(ctx, db, os.DirFS(dir), logger.Named("migrations"), true)
	require.Error(t, err)
	t.Logf("UpWithLogger returned: %v", err)
	require.Contains(t, err.Error(), "sqlstate=42P07", "error should carry the real SQLSTATE")

	// Confirm the batch really rolled back in Postgres, not just that we
	// logged "rolled back": schema_migrations must still show
	// currentVersion (phase 1's result), and neither scratch migration's
	// side effect must be durable, including the one that individually
	// succeeded before the broken one failed.
	var version int
	require.NoError(t, db.QueryRow("SELECT version FROM schema_migrations").Scan(&version))
	require.Equal(t, currentVersion, version, "schema_migrations should be unchanged after rollback")

	var scratchTableExists int
	require.NoError(t, db.QueryRow(
		"SELECT count(*) FROM information_schema.tables WHERE table_name = 'scratch_rollback'",
	).Scan(&scratchTableExists))
	require.Zero(t, scratchTableExists, "the migration that ran before the failure must be rolled back too")
}

// TestUpWithLoggerEventFieldIsAlwaysFirst guards the "event is the first
// field on every structured migration log line" invariant against a
// future refactor accidentally reordering fields. It is a positional
// check only; the scenario tests above already cover the specific event
// values and message text.
func TestUpWithLoggerEventFieldIsAlwaysFirst(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db := scenarioDB(t)
	ctx := context.Background()

	sink := testutil.NewFakeSink(t)
	logger := sink.Logger(slog.LevelDebug)
	require.NoError(t, runUp(ctx, db, nil, logger.Named("migrations"), true))

	entries := sink.Entries()
	require.NotEmpty(t, entries, "a fresh install should produce at least the inventory and outcome lines")
	for _, e := range entries {
		require.NotEmpty(t, e.Fields, "log line %q should carry fields", e.Message)
		require.Equal(t, "event", e.Fields[0].Name, "log line %q should carry event as its first field", e.Message)
	}
}

// failOpenFS wraps an fs.FS and fails every Open call for one exact path,
// simulating an unreadable migration file (a non-SQL failure) rather than
// a SQL error. It only overrides Open, so the one-time directory listing
// iofs.New does at construction (which uses fs.ReadDir, not this failing
// path) still sees the file and counts it as pending; only the later,
// on-demand read through ReadUp actually fails.
type failOpenFS struct {
	fs.FS
	failPath string
}

func (f failOpenFS) Open(name string) (fs.File, error) {
	if name == f.failPath {
		return nil, fmt.Errorf("simulated unreadable migration file: %s", name)
	}
	return f.FS.Open(name)
}

// TestUpWithLoggerCommittedBeforeFailure first migrates a scratch database
// to the real, current schema, then appends two valid migrations followed
// by one whose file cannot be read. It confirms the non-SQL failure is
// classified separately from a SQL failure (OutcomeCommittedBeforeFailure,
// not OutcomeRolledBack), and that the two valid migrations that ran
// before the read failure are durably committed, since pgTxnDriver commits
// even after a non-SQL failure.
func TestUpWithLoggerCommittedBeforeFailure(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db := scenarioDB(t)
	ctx := context.Background()

	setupLogger := scenarioLogger(t, "committed-before-failure-setup")
	require.NoError(t, runUp(ctx, db, nil, setupLogger.Named("migrations"), true))

	var currentVersion int
	require.NoError(t, db.QueryRow("SELECT version FROM schema_migrations").Scan(&currentVersion))

	dir, nextVersion := scratchMigrations(t)
	require.Equal(t, currentVersion+1, nextVersion)
	writeScratchMigration(t, dir, nextVersion, "scratch_a",
		"CREATE TABLE scratch_a (id int);\n", "DROP TABLE scratch_a;\n")
	writeScratchMigration(t, dir, nextVersion+1, "scratch_b",
		"CREATE TABLE scratch_b (id int);\n", "DROP TABLE scratch_b;\n")
	writeScratchMigration(t, dir, nextVersion+2, "unreadable",
		"CREATE TABLE scratch_c (id int);\n", "DROP TABLE scratch_c;\n")

	failPath := fmt.Sprintf("%06d_unreadable.up.sql", nextVersion+2)
	scratchFS := failOpenFS{FS: os.DirFS(dir), failPath: failPath}

	logger := scenarioLogger(t, "committed-before-failure")
	err := runUp(ctx, db, scratchFS, logger.Named("migrations"), true)
	require.Error(t, err)
	t.Logf("UpWithLogger returned: %v", err)

	// The two valid migrations before the unreadable one must be durably
	// committed: a non-SQL failure commits what already ran, it does not
	// roll back.
	var version int
	require.NoError(t, db.QueryRow("SELECT version FROM schema_migrations").Scan(&version))
	require.Equal(t, nextVersion+1, version, "the two valid migrations before the unreadable one should be committed")

	var scratchTableCount int
	require.NoError(t, db.QueryRow(
		"SELECT count(*) FROM information_schema.tables WHERE table_name IN ('scratch_a', 'scratch_b')",
	).Scan(&scratchTableCount))
	require.Equal(t, 2, scratchTableCount, "both tables created before the unreadable migration should exist")

	var thirdTableExists int
	require.NoError(t, db.QueryRow(
		"SELECT count(*) FROM information_schema.tables WHERE table_name = 'scratch_c'",
	).Scan(&thirdTableExists))
	require.Zero(t, thirdTableExists, "the unreadable migration's own table must not exist")
}

// TestUpWithLoggerDirtyDatabase flips the version table's dirty bit by
// hand (simulating an interrupted prior run) and confirms the next run
// logs "database schema is dirty" and fails before applying anything, with
// no outcome line.
func TestUpWithLoggerDirtyDatabase(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db := scenarioDB(t)
	ctx := context.Background()

	setupLogger := scenarioLogger(t, "dirty-database-setup")
	require.NoError(t, runUp(ctx, db, nil, setupLogger.Named("migrations"), true))

	_, err := db.Exec("UPDATE schema_migrations SET dirty = true")
	require.NoError(t, err)

	logger := scenarioLogger(t, "dirty-database")
	err = runUp(ctx, db, nil, logger.Named("migrations"), true)
	require.Error(t, err)
	t.Logf("UpWithLogger returned: %v", err)
}

// TestUpWithLoggerFailureBeforeFirstMigration corrupts the version table
// so the very first Version() read fails with a real query/scan error (not
// "no rows" or "undefined table", both of which are treated as a normal
// empty database), confirming a pre-flight failure also logs no outcome
// line.
func TestUpWithLoggerFailureBeforeFirstMigration(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	db := scenarioDB(t)
	ctx := context.Background()

	setupLogger := scenarioLogger(t, "pre-first-migration-failure-setup")
	require.NoError(t, runUp(ctx, db, nil, setupLogger.Named("migrations"), true))

	_, err := db.Exec("ALTER TABLE schema_migrations ALTER COLUMN version TYPE text")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE schema_migrations SET version = 'not-a-number'")
	require.NoError(t, err)

	logger := scenarioLogger(t, "pre-first-migration-failure")
	err = runUp(ctx, db, nil, logger.Named("migrations"), true)
	require.Error(t, err)
	t.Logf("UpWithLogger returned: %v", err)
}

// TestUpWithLoggerLockTimeout confirms a real finding from running this
// for real: ensureVersionTable calls Lock() directly, bypassing
// golang-migrate's LockTimeout-governed m.lock() wrapper entirely, so it
// has no timeout at all and blocks forever on contention. That call
// always runs first, on every boot, before m.Up() (and its LockTimeout)
// is ever reached. The first version of this test tried to hold the lock
// open before calling runUp and hung for the full 3-minute test timeout
// inside ensureVersionTable, confirming it live.
//
// To actually exercise the timeout golang-migrate's own LockTimeout does
// govern, this test calls setupWithLogger directly (so
// ensureVersionTable's own, unbounded lock cycle completes uncontended
// first) and only then holds the lock open before calling m.Up() itself.
// Not parallel: it overrides the package-level lockTimeout so the test
// does not have to wait out the real 2 minutes, and that override is not
// safe to race against other tests building their own driver.
func TestUpWithLoggerLockTimeout(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	db := scenarioDB(t)
	ctx := context.Background()

	setupLogger := scenarioLogger(t, "lock-timeout-setup")
	require.NoError(t, runUp(ctx, db, nil, setupLogger.Named("migrations"), true))

	previous := lockTimeout
	lockTimeout = 2 * time.Second
	t.Cleanup(func() { lockTimeout = previous })

	logger := scenarioLogger(t, "lock-timeout")
	_, driver, m, err := setupWithLogger(db, nil, logger.Named("migrations"), true)
	require.NoError(t, err, "ensureVersionTable's own lock cycle should succeed uncontended here")
	defer func() { _, _ = m.Close() }()

	holder, err := db.Conn(ctx)
	require.NoError(t, err)
	holderTx, err := holder.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = holderTx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lockID)
	require.NoError(t, err)

	start := time.Now()
	upErr := m.Up()
	t.Logf("m.Up() returned after %s: %v", time.Since(start), upErr)
	require.ErrorIs(t, upErr, migrate.ErrLockTimeout)

	// Real finding, confirmed live by this test, not introduced by this
	// change and out of scope to fix here (it is a pre-existing,
	// structural interaction between golang-migrate and pgTxnDriver, not
	// a logging concern): m.lock() races Lock() against LockTimeout
	// without cancelling the loser. Lock() is still blocked on
	// pg_advisory_xact_lock in the background even though m.Up() already
	// returned ErrLockTimeout to the caller. Once the holder below
	// releases the real lock, that abandoned call finally succeeds and
	// leaves an open transaction that nothing will ever call Unlock() on:
	// pgTxnDriver.ctx is deliberately context.Background() (so a
	// cancelled caller context can never abort a half-applied migration),
	// so there is no path left that stops its Tx.awaitDone goroutine,
	// which only reacts to that transaction's own derived context being
	// cancelled via Commit or Rollback. In production there is no way to
	// reach that abandoned driver instance at all once the caller has
	// moved on: this leaks one Postgres connection and one goroutine for
	// the rest of the process's life, every time this exact race happens.
	// This test can only avoid leaking into the rest of this binary's
	// goleak check because, as a white-box test, it still holds the
	// driver reference setupWithLogger returned, and can roll back that
	// abandoned transaction by hand once it is done blocking.
	require.NoError(t, holderTx.Rollback())
	require.NoError(t, holder.Close())
	// driver.tx is assigned the instant BeginTx succeeds, near the very
	// start of Lock(), well before the blocking advisory-lock exec that
	// follows it; it is not a signal that the abandoned call has finished
	// acquiring the lock and returned. There is no observable signal for
	// that from outside golang-migrate's own lock() goroutine, so this
	// sleep is a generous, test-only margin, not a synchronization
	// mechanism: calling Rollback concurrently with the abandoned
	// goroutine's own in-flight exec on the same *sql.Tx would itself be
	// a race, which is exactly why production code has no safe way to
	// reach in and do this at all.
	time.Sleep(time.Second)
	require.NoError(t, driver.tx.Rollback())
}

// TestUpWithLoggerReplicaWaitingOnLock holds the advisory lock open from a
// real, separate Postgres backend for 40 real seconds (long enough for one
// real 30-second heartbeat tick) while a second, fully-migrated instance
// tries to start up, confirming the "waiting"/"still waiting"/"acquired"
// sequence and that the wait is reported against a real blocking pid.
//
// Not parallel: lockID is a fixed, hardcoded production constant, and
// Postgres advisory locks are scoped to the whole server, not to a single
// database. Running this alongside another test that also holds lockID
// open (even against a different scratch database) would create false
// contention between two otherwise-unrelated tests, which is exactly what
// happened the first time this ran parallel with the relation-lock
// scenario below.
func TestUpWithLoggerReplicaWaitingOnLock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	db := scenarioDB(t)
	ctx := context.Background()

	setupLogger := scenarioLogger(t, "replica-waiting-on-lock-setup")
	require.NoError(t, runUp(ctx, db, nil, setupLogger.Named("migrations"), true))

	holder, err := db.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.Close() })
	holderTx, err := holder.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = holderTx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lockID)
	require.NoError(t, err)

	// The rollback that releases the lock must happen here, right after
	// the hold duration elapses, not after waiting on runUp below: runUp
	// cannot return until this lock is released, so rolling back only
	// after it returns would deadlock the test against itself.
	go func() {
		// Held via the query itself, not a Go-side time.Sleep, so the
		// real backend genuinely stays busy and the heartbeat's
		// pg_stat_activity query sees it holding the lock throughout.
		_, _ = holderTx.ExecContext(ctx, "SELECT pg_sleep(40)")
		_ = holderTx.Rollback()
	}()

	logger := scenarioLogger(t, "replica-waiting-on-lock")
	start := time.Now()
	err = runUp(ctx, db, nil, logger.Named("migrations"), true)
	t.Logf("UpWithLogger (second instance) returned after %s: %v", time.Since(start), err)
	require.NoError(t, err, "the second instance should succeed once the first instance's lock is released")
	require.Greater(t, time.Since(start), 30*time.Second, "should have genuinely waited past one real heartbeat tick")
}

// TestUpWithLoggerMigrationBlockedByAnotherSession brings a scratch
// database to a known state with one extra table, then runs a scratch
// migration that ALTERs that table while a second, real session holds an
// open SELECT against it, so the migration's own DDL statement genuinely
// blocks on a real relation lock (not the advisory lock, which it already
// holds). Confirms the heartbeat distinguishes this from an advisory-lock
// wait: "database migration still running" with wait_event=relation and
// the real blocking pid.
//
// Not parallel: every migration run, including this one, first acquires
// the same fixed, hardcoded lockID advisory lock the previous scenario
// exercises directly; see that test's comment for why these cannot race
// each other.
func TestUpWithLoggerMigrationBlockedByAnotherSession(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	db := scenarioDB(t)
	ctx := context.Background()

	// Phase 1: real chain, plus one scratch migration that durably
	// creates the table the scenario will contend on.
	dir, nextVersion := scratchMigrations(t)
	writeScratchMigration(t, dir, nextVersion, "lockable",
		"CREATE TABLE lockable (id int);\n", "DROP TABLE lockable;\n")
	setupLogger := scenarioLogger(t, "migration-blocked-setup")
	require.NoError(t, runUp(ctx, db, os.DirFS(dir), setupLogger.Named("migrations"), true))

	// Phase 2: the scenario. One more scratch migration appended that
	// ALTERs the table created above.
	writeScratchMigration(t, dir, nextVersion+1, "alter_lockable",
		"ALTER TABLE lockable ADD COLUMN extra int;\n", "ALTER TABLE lockable DROP COLUMN extra;\n")

	blocker, err := db.Conn(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocker.Close() })
	blockerTx, err := blocker.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = blockerTx.ExecContext(ctx, "SELECT * FROM lockable")
	require.NoError(t, err)

	// As above: the rollback that releases the SELECT's lock must happen
	// right after the hold duration elapses, not after waiting on runUp
	// below, or the test deadlocks against itself.
	go func() {
		_, _ = blockerTx.ExecContext(ctx, "SELECT pg_sleep(40)")
		_ = blockerTx.Rollback()
	}()

	logger := scenarioLogger(t, "migration-blocked")
	start := time.Now()
	err = runUp(ctx, db, os.DirFS(dir), logger.Named("migrations"), true)
	t.Logf("UpWithLogger returned after %s: %v", time.Since(start), err)
	require.NoError(t, err, "the migration should succeed once the blocking session releases its SELECT")
	require.Greater(t, time.Since(start), 30*time.Second, "should have genuinely waited past one real heartbeat tick")
}

// TestUpWithLoggerSlowButUnblockedMigrationReportsRunning is a regression
// check for a real bug found during review: a genuinely slow but
// unblocked migration (pg_sleep, wait_event_type "Timeout") was
// misreported as state=waiting_on_lock, because the original check
// treated any non-empty wait_event_type as a lock wait. Only Postgres's
// "Lock" category actually is one; this confirms the fix reports
// state=running instead, with no wait_event or blocked_by, for such a
// migration.
func TestUpWithLoggerSlowButUnblockedMigrationReportsRunning(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	db := scenarioDB(t)
	ctx := context.Background()

	dir, nextVersion := scratchMigrations(t)
	writeScratchMigration(t, dir, nextVersion, "slow_unblocked",
		"SELECT pg_sleep(35);\n", "SELECT 1;\n")

	logger := scenarioLogger(t, "slow-unblocked-reports-running")
	start := time.Now()
	err := runUp(ctx, db, os.DirFS(dir), logger.Named("migrations"), true)
	t.Logf("UpWithLogger returned after %s: %v", time.Since(start), err)
	require.NoError(t, err)
}
