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
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/lib/pq"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/retry"
)

//go:embed *.sql
var migrations embed.FS

const (
	// LockTimeoutEnv overrides the Postgres lock_timeout applied to the
	// migration transaction. It accepts a Go duration such as "5s". A value
	// of 0 disables the timeout so migrations wait for locks indefinitely.
	LockTimeoutEnv = "CODER_PG_MIGRATION_LOCK_TIMEOUT"
	// DefaultLockTimeout is the lock_timeout used when LockTimeoutEnv is not
	// set. Migrations that cannot acquire a relation lock within this window
	// fail and are retried instead of queueing behind application traffic,
	// where their pending DDL would in turn block every later query on the
	// same relation.
	DefaultLockTimeout = 3 * time.Second

	// maxAttempts bounds how many times a migration batch is retried after
	// losing a lock race.
	maxAttempts = 5

	// SQLSTATE codes that indicate the batch lost a lock race rather than
	// hit a genuine migration bug.
	pgCodeLockNotAvailable pq.ErrorCode = "55P03"
	pgCodeDeadlockDetected pq.ErrorCode = "40P01"
)

// Option configures Up, UpWithFS, and Down.
type Option func(*options)

type options struct {
	ctx    context.Context
	logger slog.Logger
	// lockTimeout is only honored when lockTimeoutSet is true; otherwise it is
	// resolved from LockTimeoutEnv or DefaultLockTimeout.
	lockTimeout    time.Duration
	lockTimeoutSet bool
}

// WithContext bounds the migration run, including retry backoff, by ctx.
func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.ctx = ctx
	}
}

// WithLogger sets the logger used to report lock retries.
func WithLogger(logger slog.Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

// WithLockTimeout sets the Postgres lock_timeout for the migration
// transaction, overriding LockTimeoutEnv. Zero waits for locks indefinitely.
func WithLockTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.lockTimeout = timeout
		o.lockTimeoutSet = true
	}
}

func newOptions(opts []Option) (options, error) {
	o := options{
		ctx:    context.Background(),
		logger: slog.Make(sloghuman.Sink(os.Stderr)).Named("migrations"),
	}
	for _, opt := range opts {
		opt(&o)
	}
	if o.lockTimeoutSet {
		return o, nil
	}
	o.lockTimeout = DefaultLockTimeout
	if raw, ok := os.LookupEnv(LockTimeoutEnv); ok {
		timeout, err := time.ParseDuration(raw)
		if err != nil {
			return o, xerrors.Errorf("parse %s=%q: %w", LockTimeoutEnv, raw, err)
		}
		if timeout < 0 {
			return o, xerrors.Errorf("parse %s=%q: must not be negative", LockTimeoutEnv, raw)
		}
		o.lockTimeout = timeout
	}
	return o, nil
}

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

func setup(db *sql.DB, migs fs.FS, opts options) (source.Driver, *migrate.Migrate, error) {
	if migs == nil {
		migs = migrations
	}
	ctx := opts.ctx
	sourceDriver, err := iofs.New(migs, ".")
	if err != nil {
		return nil, nil, xerrors.Errorf("create iofs: %w", err)
	}

	// migration_cursor is a v1 migration table. If this exists, we're on v1.
	// Do no run v2 migrations on a v1 database!
	row := db.QueryRowContext(ctx, "SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'migration_cursor';")
	var v1Exists int
	if row.Scan(&v1Exists) == nil {
		return nil, nil, xerrors.New("currently connected to a Coder v1 database, aborting database setup")
	}

	dbDriver := &pgDriver{ctx: ctx, db: db, lockTimeout: opts.lockTimeout}
	err = dbDriver.ensureVersionTable()
	if err != nil {
		return nil, nil, xerrors.Errorf("ensure version table: %w", err)
	}

	m, err := migrate.NewWithInstance("", sourceDriver, "", dbDriver)
	if err != nil {
		return nil, nil, xerrors.Errorf("new migrate instance: %w", err)
	}

	// The default LockTimeout of 15s is too short for concurrent migrations,
	// especially when the number of migrations is large. Since we use
	// pg_advisory_xact_lock which releases automatically when the transaction
	// ends, we just need to wait long enough for any concurrent migration to
	// finish.
	m.LockTimeout = 2 * time.Minute

	return sourceDriver, m, nil
}

// Up runs SQL migrations to ensure the database schema is up-to-date.
func Up(db *sql.DB) error {
	return UpWithFS(db, migrations)
}

// UpWithFS runs SQL migrations in the given fs.
//
// Each pending migration commits in its own transaction together with its
// schema_migrations row, under a bounded Postgres lock_timeout, while a batch
// level advisory lock serializes concurrent replicas. If a migration fails
// because it lost a lock race, it is rolled back and the remaining migrations
// are retried from the last committed version with exponential backoff. Any
// other failure is returned immediately and leaves the already committed
// migrations in place; a later call resumes from that version.
func UpWithFS(db *sql.DB, migs fs.FS, opts ...Option) error {
	o, err := newOptions(opts)
	if err != nil {
		return err
	}
	return runWithLockRetry(o, "up", func() error {
		return upOnce(db, migs, o)
	})
}

func upOnce(db *sql.DB, migs fs.FS, opts options) (retErr error) {
	_, m, err := setup(db, migs, opts)
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

	err = m.Up()
	if err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			// It's OK if no changes happened!
			return nil
		}

		return xerrors.Errorf("up: %w", err)
	}

	return nil
}

// Down runs all down SQL migrations. It retries lost lock races the same way
// UpWithFS does.
func Down(db *sql.DB, opts ...Option) error {
	o, err := newOptions(opts)
	if err != nil {
		return err
	}
	return runWithLockRetry(o, "down", func() error {
		return downOnce(db, o)
	})
}

func downOnce(db *sql.DB, opts options) error {
	_, m, err := setup(db, migrations, opts)
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

// runWithLockRetry runs one migration batch attempt at a time until it
// succeeds, fails for a reason other than lock contention, exhausts
// maxAttempts, or opts.ctx is done. Each attempt rebuilds its own driver
// because pgDriver.Unlock releases the batch connection, and resumes from the
// version the previous attempt committed.
func runWithLockRetry(opts options, direction string, attemptFn func() error) error {
	backoff := retry.New(250*time.Millisecond, 5*time.Second)
	backoff.Jitter = 0.2

	var lastErr error
	// The first Wait returns immediately, so the first attempt is not delayed.
	for attempt := 1; backoff.Wait(opts.ctx); attempt++ {
		lastErr = attemptFn()
		if lastErr == nil {
			return nil
		}
		pgErr, ok := lockContentionError(lastErr)
		if !ok {
			return lastErr
		}
		if attempt >= maxAttempts {
			return xerrors.Errorf("migration %s lost lock race on all %d attempts: %w", direction, attempt, lastErr)
		}
		opts.logger.Warn(opts.ctx, "database migration lost a lock race and was rolled back, retrying remaining migrations",
			slog.F("direction", direction),
			slog.F("attempt", attempt),
			slog.F("max_attempts", maxAttempts),
			slog.F("lock_timeout", opts.lockTimeout),
			slog.F("sqlstate", pgErr.Code),
			slog.F("message", pgErr.Message),
			slog.F("detail", pgErr.Detail),
			slog.F("relation", pgErr.Table),
			slog.Error(lastErr),
		)
	}
	return xerrors.Errorf("migration %s canceled while waiting to retry after %v: %w", direction, lastErr, opts.ctx.Err())
}

// lockContentionError returns the Postgres error behind err when it reports
// that a statement lost a lock race: lock_not_available (55P03), raised when
// lock_timeout expires, or deadlock_detected (40P01).
//
// golang-migrate wraps driver failures in database.Error, which does not
// implement Unwrap, so the chain is walked through OrigErr explicitly.
func lockContentionError(err error) (*pq.Error, bool) {
	pgErr, ok := pqError(err)
	if !ok {
		return nil, false
	}
	switch pgErr.Code {
	case pgCodeLockNotAvailable, pgCodeDeadlockDetected:
		return pgErr, true
	default:
		return nil, false
	}
}

func pqError(err error) (*pq.Error, bool) {
	for err != nil {
		var pgErr *pq.Error
		if xerrors.As(err, &pgErr) {
			return pgErr, true
		}
		var dbErrPtr *database.Error
		if xerrors.As(err, &dbErrPtr) && dbErrPtr != nil {
			err = dbErrPtr.OrigErr
			continue
		}
		var dbErr database.Error
		if xerrors.As(err, &dbErr) {
			err = dbErr.OrigErr
			continue
		}
		return nil, false
	}
	return nil, false
}

// EnsureClean checks whether all migrations for the current version have been
// applied, without making any changes to the database. If not, returns a
// non-nil error.
func EnsureClean(db *sql.DB) error {
	opts, err := newOptions(nil)
	if err != nil {
		return err
	}
	sourceDriver, m, err := setup(db, migrations, opts)
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
	opts, err := newOptions(nil)
	if err != nil {
		return nil, err
	}
	_, m, err := setup(db, migrations, opts)
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
