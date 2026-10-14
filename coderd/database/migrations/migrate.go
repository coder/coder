package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

//go:embed *.sql
var migrations embed.FS

var (
	migrationsHash     string
	migrationsHashOnce sync.Once
)

// A migrations hash is a sha256 hash of the contents and names
// of the migrations sorted by filename.
func calculateMigrationsHash(migrationsFs embed.FS) (string, error) {
	files, err := migrationsFs.ReadDir(".")
	if err != nil {
		return "", xerrors.Errorf("read migrations directory: %w", err)
	}
	sortedFiles := make([]fs.DirEntry, len(files))
	copy(sortedFiles, files)
	sort.Slice(sortedFiles, func(i, j int) bool {
		return sortedFiles[i].Name() < sortedFiles[j].Name()
	})

	var builder strings.Builder
	for _, file := range sortedFiles {
		if _, err := builder.WriteString(file.Name()); err != nil {
			return "", xerrors.Errorf("write migration file name %q: %w", file.Name(), err)
		}
		content, err := migrationsFs.ReadFile(file.Name())
		if err != nil {
			return "", xerrors.Errorf("read migration file %q: %w", file.Name(), err)
		}
		if _, err := builder.Write(content); err != nil {
			return "", xerrors.Errorf("write migration file content %q: %w", file.Name(), err)
		}
	}

	hash := sha256.New()
	if _, err := hash.Write([]byte(builder.String())); err != nil {
		return "", xerrors.Errorf("write to hash: %w", err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func GetMigrationsHash() string {
	migrationsHashOnce.Do(func() {
		hash, err := calculateMigrationsHash(migrations)
		if err != nil {
			panic(err)
		}
		migrationsHash = hash
	})
	return migrationsHash
}

func setup(db *sql.DB, migs fs.FS) (source.Driver, *migrate.Migrate, error) {
	sourceDriver, _, m, err := setupWithLogger(db, migs, slog.Logger{}, false)
	return sourceDriver, m, err
}

// setupWithLogger is setup plus an instrumented driver: logger receives
// structured progress and outcome events, and diagnostics gates the
// heartbeat's pg_stat_activity query so plain Up/UpWithFS callers (which
// pass the zero value logger and diagnostics=false) issue no extra queries.
func setupWithLogger(db *sql.DB, migs fs.FS, logger slog.Logger, diagnostics bool) (source.Driver, *pgTxnDriver, *migrate.Migrate, error) {
	if migs == nil {
		migs = migrations
	}
	ctx := context.Background()
	sourceDriver, err := iofs.New(migs, ".")
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("create iofs: %w", err)
	}

	// migration_cursor is a v1 migration table. If this exists, we're on v1.
	// Do no run v2 migrations on a v1 database!
	row := db.QueryRowContext(ctx, "SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'migration_cursor';")
	var v1Exists int
	if row.Scan(&v1Exists) == nil {
		return nil, nil, nil, xerrors.New("currently connected to a Coder v1 database, aborting database setup")
	}

	dbDriver := &pgTxnDriver{
		ctx:         context.Background(),
		db:          db,
		logger:      logger,
		diagnostics: diagnostics,
		source:      sourceDriver,
	}
	err = dbDriver.ensureVersionTable()
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("ensure version table: %w", err)
	}

	m, err := migrate.NewWithInstance("", sourceDriver, "", dbDriver)
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("new migrate instance: %w", err)
	}

	// The default LockTimeout of 15s is too short for concurrent migrations,
	// especially when the number of migrations is large. Since we use
	// pg_advisory_xact_lock which releases automatically when the transaction
	// ends, we just need to wait long enough for any concurrent migration to
	// finish. A package variable, not a literal, so tests can exercise the
	// lock-timeout path without waiting the real duration out.
	m.LockTimeout = lockTimeout

	return sourceDriver, dbDriver, m, nil
}

// lockTimeout is how long a migration run waits to acquire the advisory
// lock before giving up. Overridden only by tests, and only ones that do
// not run in parallel with any other test in this package.
var lockTimeout = 2 * time.Minute

// Up runs SQL migrations to ensure the database schema is up-to-date.
func Up(db *sql.DB) error {
	return UpWithFS(db, migrations)
}

// UpWithFS runs SQL migrations in the given fs.
func UpWithFS(db *sql.DB, migs fs.FS) error {
	return runUp(context.Background(), db, migs, slog.Logger{}, false)
}

// UpWithLogger runs SQL migrations the same way Up does, additionally
// reporting structured progress and outcome events to logger, named
// "migrations" by convention.
func UpWithLogger(db *sql.DB, logger slog.Logger) error {
	return runUp(context.Background(), db, migrations, logger, true)
}

// runUp is the shared implementation behind Up, UpWithFS and UpWithLogger.
// diagnostics gates the heartbeat's pg_stat_activity query and the outcome
// log line, so plain Up/UpWithFS callers behave exactly as before.
func runUp(ctx context.Context, db *sql.DB, migs fs.FS, logger slog.Logger, diagnostics bool) (retErr error) {
	_, driver, m, err := setupWithLogger(db, migs, logger, diagnostics)
	if err != nil {
		return xerrors.Errorf("migrate setup: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if retErr != nil {
			return
		}
		if dbErr != nil {
			retErr = dbErr
			return
		}
		retErr = srcErr
	}()

	mUpErr := m.Up()
	if mUpErr != nil && errors.Is(mUpErr, migrate.ErrNoChange) {
		// It's OK if no changes happened!
		mUpErr = nil
	}

	if !diagnostics {
		if mUpErr != nil {
			return xerrors.Errorf("up: %w", mUpErr)
		}
		return nil
	}

	var finalErr error
	if mUpErr != nil {
		finalErr = driver.wrapFailure(mUpErr)
	}
	logOutcome(ctx, logger, driver, finalErr)

	if finalErr != nil {
		return xerrors.Errorf("up: %w", finalErr)
	}
	return nil
}

// Outcome classifies how a migration run concluded.
type Outcome int

const (
	// OutcomeNoChange means no migration was applied: the database was
	// already current, or the run failed before the first migration (a
	// dirty database, a lock timeout, or a pre-flight error).
	OutcomeNoChange Outcome = iota
	// OutcomeCommitted means every migration in the batch committed.
	OutcomeCommitted
	// OutcomeRolledBack means a migration's SQL failed, and PostgreSQL
	// rolled the whole batch back; the schema version is unchanged.
	OutcomeRolledBack
	// OutcomeCommittedBeforeFailure means the batch committed, but a later,
	// non-SQL failure (such as an unreadable migration file) stopped the
	// run before it reached the target version.
	OutcomeCommittedBeforeFailure
	// OutcomeUnknown means the commit's result could not be classified.
	OutcomeUnknown
)

// Event is the stable, first field on every structured migration log line.
// Phase 2's Prometheus metrics and telemetry reuse this type instead of
// redefining event names.
type Event string

const (
	EventInventoryPending              Event = "db_migration_inventory_pending"
	EventInventoryUpToDate             Event = "db_migration_inventory_up_to_date"
	EventInventoryDirty                Event = "db_migration_inventory_dirty"
	EventLockWaitStarted               Event = "db_migration_lock_wait_started"
	EventLockAcquired                  Event = "db_migration_lock_acquired"
	EventStepStarted                   Event = "db_migration_step_started"
	EventStepApplied                   Event = "db_migration_step_applied"
	EventHeartbeat                     Event = "db_migration_heartbeat"
	EventOutcomeNoChange               Event = "db_migration_outcome_no_change"
	EventOutcomeDirty                  Event = "db_migration_outcome_dirty"
	EventOutcomeCommitted              Event = "db_migration_outcome_committed"
	EventOutcomeRolledBack             Event = "db_migration_outcome_rolled_back"
	EventOutcomeCommittedBeforeFailure Event = "db_migration_outcome_committed_before_failure"
	EventOutcomeLockTimeout            Event = "db_migration_outcome_lock_timeout"
	EventOutcomeUnknown                Event = "db_migration_outcome_unknown"
)

// Event derives the log event from the outcome itself, for outcomes with
// exactly one meaning. OutcomeNoChange covers three different situations
// (genuinely up to date, a lock timeout, or a database already known to be
// dirty or otherwise unreadable), so its logging goes through the specific
// event constant for that situation directly, not through this method.
func (o Outcome) Event() Event {
	switch o {
	case OutcomeCommitted:
		return EventOutcomeCommitted
	case OutcomeRolledBack:
		return EventOutcomeRolledBack
	case OutcomeCommittedBeforeFailure:
		return EventOutcomeCommittedBeforeFailure
	case OutcomeUnknown:
		return EventOutcomeUnknown
	default:
		return ""
	}
}

// Applied records one migration that ran during a Result.
type Applied struct {
	Version   int
	Name      string
	StartedAt time.Time
	Duration  time.Duration
}

// Failure records the migration a Result failed on, if any.
type Failure struct {
	Version  int
	Name     string
	SQLState string
	Err      error
}

// Result is a snapshot of one migration run. Nothing outside the package
// needs it yet; the telemetry phase persists this same shape as the run
// record.
type Result struct {
	StartedAt, FinishedAt time.Time
	FromVersion           int // -1 on an empty database
	ToVersion             int
	Applied               []Applied
	Outcome               Outcome
	Failed                *Failure
}

// logOutcome classifies and logs how the run concluded, and returns the
// Result it derived that from. It runs after m.Up() returns, because only
// then do we know both the driver's commit result and whether a later,
// non-SQL failure (such as an unreadable migration file) happened despite a
// clean commit.
func logOutcome(ctx context.Context, logger slog.Logger, d *pgTxnDriver, finalErr error) Result {
	d.mu.Lock()
	fromVersion := d.fromVersion
	runStart := d.runStart
	applied := append([]Applied(nil), d.applied...)
	commitErr := d.commitErr
	sqlState := d.sqlState
	inFlightVersion, inFlightName := d.inFlight, d.inFlightName
	inFlightStart := d.inFlightStart
	d.mu.Unlock()

	result := Result{FromVersion: fromVersion, ToVersion: fromVersion, Applied: applied}

	if len(applied) == 0 {
		result.Outcome = OutcomeNoChange
		switch {
		case finalErr == nil:
			// Genuinely nothing pending: Version() found the schema
			// already current.
			logger.Info(ctx, "database migration outcome: no change",
				slog.F("event", EventOutcomeNoChange), slog.F("status", "success"),
				slog.F("count", 0), slog.F("schema_version", fromVersion),
				slog.F("total_duration", time.Since(runStart)))

		case errors.Is(finalErr, migrate.ErrLockTimeout):
			// golang-migrate's own LockTimeout gave up waiting for the
			// advisory lock before Lock() itself returned; Lock() has no
			// hook for this, so this is the first point we learn of it.
			// The timeout firing is a confirmed fact, not a possibility.
			result.Failed = &Failure{Version: inFlightVersion, Name: inFlightName, SQLState: sqlState, Err: finalErr}
			logger.Error(ctx, "gave up waiting for database migration lock",
				slog.F("event", EventOutcomeLockTimeout), slog.F("status", "failure"),
				slog.F("count", 0), slog.F("waited", time.Since(runStart)))

		case errors.As(finalErr, &migrate.ErrDirty{}):
			// The database is confirmed dirty, logged again here (in
			// addition to logPending's own line) so every run concludes
			// with exactly one outcome line, regardless of what happened.
			result.Failed = &Failure{Version: inFlightVersion, Name: inFlightName, SQLState: sqlState, Err: finalErr}
			logger.Error(ctx, "database migration outcome: dirty schema",
				slog.F("event", EventOutcomeDirty), slog.F("status", "failure"),
				slog.F("count", 0), slog.F("schema_version", fromVersion),
				slog.F("total_duration", time.Since(runStart)),
				slog.F("error", fmt.Sprintf("dirty database version %d", fromVersion)))

		case errors.Is(commitErr, pq.ErrInFailedTransaction):
			// The first pending migration's own SQL failed and the
			// (otherwise empty) transaction rolled back: the same event
			// as the len(applied)>0 case below, just with nothing having
			// succeeded first.
			result.Outcome = OutcomeRolledBack
			result.Failed = &Failure{Version: inFlightVersion, Name: inFlightName, SQLState: sqlState, Err: finalErr}
			logger.Error(ctx, "rolled back database migrations",
				slog.F("event", EventOutcomeRolledBack), slog.F("status", "failure"),
				slog.F("count", 0), slog.F("schema_version", fromVersion),
				slog.F("total_duration", time.Since(runStart)), slog.F("failed_duration", time.Since(inFlightStart)))

		default:
			// A bare failure before the first migration (reading the
			// version table itself failed): the error alone says enough,
			// no dedicated event for this one.
			result.Failed = &Failure{Version: inFlightVersion, Name: inFlightName, SQLState: sqlState, Err: finalErr}
		}
		return result
	}

	last := applied[len(applied)-1]
	// Anchored at runStart (the batch Lock() call), not the first applied
	// migration's own start, so this covers any lock-wait before the first
	// migration and the final commit, the same span every len(applied)==0
	// branch above already measures. One formula, one meaning, for every
	// outcome's total_duration.
	totalDuration := time.Since(runStart)

	switch {
	case commitErr == nil && finalErr == nil:
		result.Outcome = OutcomeCommitted
		result.ToVersion = last.Version
		logger.Info(ctx, "committed database migrations",
			slog.F("event", result.Outcome.Event()), slog.F("status", "success"),
			slog.F("count", len(applied)), slog.F("from_version", fromVersion),
			slog.F("to_version", last.Version), slog.F("total_duration", totalDuration))

	case commitErr == nil && finalErr != nil:
		// The failed migration (reading its source file) never started,
		// so there is no in-flight duration of its own to report here.
		result.Outcome = OutcomeCommittedBeforeFailure
		result.ToVersion = last.Version
		result.Failed = &Failure{Version: inFlightVersion, Name: inFlightName, SQLState: sqlState, Err: finalErr}
		logger.Error(ctx, "committed database migrations before failure",
			slog.F("event", result.Outcome.Event()), slog.F("status", "failure"),
			slog.F("count", len(applied)), slog.F("from_version", fromVersion),
			slog.F("to_version", last.Version), slog.F("total_duration", totalDuration))

	case errors.Is(commitErr, pq.ErrInFailedTransaction):
		// The failing migration's own SQL ran for a real amount of time
		// before it failed; failed_duration is how long, so total_duration
		// and the applied migrations' own durations don't leave an
		// unexplained gap for whoever reads this line.
		result.Outcome = OutcomeRolledBack
		result.ToVersion = fromVersion
		result.Failed = &Failure{Version: inFlightVersion, Name: inFlightName, SQLState: sqlState, Err: finalErr}
		logger.Error(ctx, "rolled back database migrations",
			slog.F("event", result.Outcome.Event()), slog.F("status", "failure"),
			slog.F("count", len(applied)), slog.F("schema_version", fromVersion),
			slog.F("total_duration", totalDuration), slog.F("failed_duration", time.Since(inFlightStart)))

	default:
		// The commit's own result could not be classified: this is a
		// genuine "might have failed" case, not a confirmed one.
		result.Outcome = OutcomeUnknown
		result.Failed = &Failure{Version: inFlightVersion, Name: inFlightName, SQLState: sqlState, Err: finalErr}
		logger.Warn(ctx, "database migration commit outcome unknown",
			slog.F("event", result.Outcome.Event()), slog.F("status", "failure"),
			slog.F("count", len(applied)), slog.F("version", fromVersion),
			slog.F("total_duration", totalDuration), slog.Error(commitErr))
	}

	return result
}

// wrapFailure turns a failed m.Up() into the concise error an operator
// sees: it names the failed migration and its SQLSTATE, keeps the
// PostgreSQL message and detail, and drops the SQL body and the always-zero
// line number that golang-migrate's database.Error carries (it has no
// Unwrap, so those fields are captured directly on the driver instead of
// recovered from the error chain).
func (d *pgTxnDriver) wrapFailure(upErr error) error {
	d.mu.Lock()
	version, name := d.inFlight, d.inFlightName
	message := d.failureMessage
	sqlState := d.sqlState
	d.mu.Unlock()

	if version < 0 {
		// A non-SQL failure, such as an unreadable migration file: no
		// migration was marked in-flight, so there is nothing to name.
		return upErr
	}
	if message == "" {
		message = upErr.Error()
	}
	if sqlState != "" {
		message = fmt.Sprintf("%s (sqlstate=%s)", message, sqlState)
	}
	return xerrors.Errorf("migration %d (%s): %s", version, name, message)
}

// Down runs all down SQL migrations.
func Down(db *sql.DB) error {
	_, m, err := setup(db, migrations)
	if err != nil {
		return xerrors.Errorf("migrate setup: %w", err)
	}

	err = m.Down()
	if err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			// It's OK if no changes happened!
			return nil
		}

		return xerrors.Errorf("down: %w", err)
	}

	return nil
}

// EnsureClean checks whether all migrations for the current version have been
// applied, without making any changes to the database. If not, returns a
// non-nil error.
func EnsureClean(db *sql.DB) error {
	sourceDriver, m, err := setup(db, migrations)
	if err != nil {
		return xerrors.Errorf("migrate setup: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil {
		return xerrors.Errorf("get migration version: %w", err)
	}

	if dirty {
		return xerrors.Errorf("database has not been cleanly migrated")
	}

	// Verify that the database's migration version is "current" by checking
	// that a migration with that version exists, but there is no next version.
	err = CheckLatestVersion(sourceDriver, version)
	if err != nil {
		return xerrors.Errorf("database needs migration: %w", err)
	}

	return nil
}

// Returns nil if currentVersion corresponds to the latest available migration,
// otherwise an error explaining why not.
func CheckLatestVersion(sourceDriver source.Driver, currentVersion uint) error {
	// This is ugly, but seems like the only way to do it with the public
	// interfaces provided by golang-migrate.

	// Check that there is no later version
	nextVersion, err := sourceDriver.Next(currentVersion)
	if err == nil {
		return xerrors.Errorf("current version is %d, but later version %d exists", currentVersion, nextVersion)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return xerrors.Errorf("get next migration after %d: %w", currentVersion, err)
	}

	// Once we reach this point, we know that either currentVersion doesn't
	// exist, or it has no successor (the return value from
	// sourceDriver.Next() is the same in either case). So we need to check
	// that either it's the first version, or it has a predecessor.

	firstVersion, err := sourceDriver.First()
	if err != nil {
		// the total number of migrations should be non-zero, so this must be
		// an actual error, not just a missing file
		return xerrors.Errorf("get first migration: %w", err)
	}
	if firstVersion == currentVersion {
		return nil
	}

	_, err = sourceDriver.Prev(currentVersion)
	if err != nil {
		return xerrors.Errorf("get previous migration: %w", err)
	}
	return nil
}

// Stepper returns a function that runs SQL migrations one step at a time.
//
// Stepper cannot be closed pre-emptively, it must be run to completion
// (or until an error is encountered).
func Stepper(db *sql.DB) (next func() (version uint, more bool, err error), err error) {
	_, m, err := setup(db, migrations)
	if err != nil {
		return nil, xerrors.Errorf("migrate setup: %w", err)
	}

	return func() (version uint, more bool, err error) {
		defer func() {
			if !more {
				srcErr, dbErr := m.Close()
				if err != nil {
					return
				}
				if dbErr != nil {
					err = dbErr
					return
				}
				err = srcErr
			}
		}()

		err = m.Steps(1)
		if err != nil {
			switch {
			case errors.Is(err, migrate.ErrNoChange):
				// It's OK if no changes happened!
				return 0, false, nil
			case errors.Is(err, fs.ErrNotExist):
				// This error is encountered at the of Steps when
				// reading from embed.FS.
				return 0, false, nil
			}

			return 0, false, xerrors.Errorf("Step: %w", err)
		}

		v, _, err := m.Version()
		if err != nil {
			return 0, false, err
		}

		return v, true, nil
	}, nil
}
