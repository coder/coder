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

func setup(ctx context.Context, db *sql.DB, migs fs.FS, logger slog.Logger) (source.Driver, *pgTxnDriver, *migrate.Migrate, error) {
	if migs == nil {
		migs = migrations
	}
	sourceDriver, err := iofs.New(migs, ".")
	if err != nil {
		return nil, nil, nil, xerrors.Errorf("create iofs: %w", err)
	}

	// migration_cursor is a v1 migration table. If this exists, we're on v1.
	// Do no run v2 migrations on a v1 database!
	row := db.QueryRowContext(context.Background(), "SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'migration_cursor';")
	var v1Exists int
	if row.Scan(&v1Exists) == nil {
		return nil, nil, nil, xerrors.New("currently connected to a Coder v1 database, aborting database setup")
	}

	dbDriver := &pgTxnDriver{ctx: context.Background(), db: db, logger: logger, logCtx: ctx, source: sourceDriver, inFlight: -1}
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
	// finish.
	m.LockTimeout = 2 * time.Minute

	return sourceDriver, dbDriver, m, nil
}

// Up runs SQL migrations to ensure the database schema is up-to-date.
func Up(db *sql.DB) error {
	return UpWithFS(db, migrations)
}

// UpWithLogger runs SQL migrations like Up and logs the progress of each
// migration along with whether the batch was committed or rolled back.
func UpWithLogger(ctx context.Context, db *sql.DB, logger slog.Logger) error {
	return runUp(ctx, db, migrations, logger)
}

// UpWithFS runs SQL migrations in the given fs.
func UpWithFS(db *sql.DB, migs fs.FS) error {
	return runUp(context.Background(), db, migs, slog.Make())
}

func runUp(ctx context.Context, db *sql.DB, migs fs.FS, logger slog.Logger) (retErr error) {
	sourceDriver, dbDriver, m, err := setup(ctx, db, migs, logger)
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

	// This check runs outside the migration lock, so a concurrent replica may
	// apply the pending migrations first. The summary logged after m.Up is
	// authoritative.
	currentVersion := -1
	if v, _, err := m.Version(); err == nil {
		currentVersion = int(v) //nolint:gosec // Migration versions are small sequential numbers.
	} else if !errors.Is(err, migrate.ErrNilVersion) {
		return xerrors.Errorf("get migration version: %w", err)
	}
	targetVersion, pending, err := pendingMigrations(sourceDriver, currentVersion)
	if err != nil {
		return xerrors.Errorf("count pending migrations: %w", err)
	}
	if pending == 0 {
		logger.Info(ctx, "database schema is up to date", slog.F("version", currentVersion))
	} else {
		logger.Info(ctx, "database migrations required", append(schemaVersionFields("current_version", currentVersion),
			slog.F("target_version", targetVersion),
			slog.F("pending", pending),
		)...)
	}

	start := time.Now()
	err = m.Up()
	if err == nil || errors.Is(err, migrate.ErrNoChange) {
		if len(dbDriver.applied) > 0 {
			logger.Info(ctx, "committed database migrations",
				slog.F("count", len(dbDriver.applied)),
				slog.F("from_version", dbDriver.applied[0]),
				slog.F("to_version", dbDriver.applied[len(dbDriver.applied)-1]),
				slog.F("duration", time.Since(start)),
			)
		}
		return nil
	}

	failFields := []slog.Field{slog.Error(err)}
	if dbDriver.inFlight >= 0 {
		failFields = append(failFields,
			slog.F("version", dbDriver.inFlight),
			slog.F("name", dbDriver.migrationName(dbDriver.inFlight)),
		)
	}
	logger.Error(ctx, "database migration failed", failFields...)

	// Nothing was written in the migration transaction, or the lock was never
	// acquired, so there is no commit outcome to report.
	if len(dbDriver.applied) == 0 && dbDriver.inFlight < 0 {
		return xerrors.Errorf("up: %w", err)
	}
	// golang-migrate commits in Unlock even after a failure. A SQL error aborts
	// the transaction, so Postgres rolls it back. Any other error, such as a
	// failure to read the next migration, leaves the transaction healthy and
	// the work done so far is committed.
	outcomeFields := []slog.Field{slog.F("count", len(dbDriver.applied))}
	if len(dbDriver.applied) > 0 {
		outcomeFields = append(outcomeFields,
			slog.F("from_version", dbDriver.applied[0]),
			slog.F("to_version", dbDriver.applied[len(dbDriver.applied)-1]),
		)
	}
	switch {
	case errors.Is(dbDriver.commitErr, pq.ErrInFailedTransaction):
		logger.Warn(ctx, "rolled back database migrations",
			append(outcomeFields, schemaVersionFields("schema_version", currentVersion)...)...)
	case dbDriver.commitErr == nil:
		if dbDriver.inFlight >= 0 {
			// The failed migration's dirty marker was committed as well.
			outcomeFields = append(outcomeFields, slog.F("dirty_version", dbDriver.inFlight))
		}
		logger.Warn(ctx, "committed database migrations before failure", outcomeFields...)
	default:
		logger.Warn(ctx, "database migration commit outcome unknown",
			append(outcomeFields, slog.Error(dbDriver.commitErr))...)
	}
	return xerrors.Errorf("up: %w", err)
}

// schemaVersionFields returns log fields describing a schema version, where -1
// means no migrations have been applied.
func schemaVersionFields(name string, version int) []slog.Field {
	if version < 0 {
		return []slog.Field{slog.F("fresh_database", true)}
	}
	return []slog.Field{slog.F("fresh_database", false), slog.F(name, version)}
}

// pendingMigrations returns the latest migration version in sourceDriver and
// the number of migrations newer than currentVersion. A currentVersion of -1
// means no migrations have been applied.
func pendingMigrations(sourceDriver source.Driver, currentVersion int) (latest int, pending int, err error) {
	v, err := sourceDriver.First()
	for err == nil {
		latest = int(v) //nolint:gosec // Migration versions are small sequential numbers.
		if latest > currentVersion {
			pending++
		}
		v, err = sourceDriver.Next(v)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return 0, 0, err
	}
	return latest, pending, nil
}

// Down runs all down SQL migrations.
func Down(db *sql.DB) error {
	_, _, m, err := setup(context.Background(), db, migrations, slog.Make())
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
	sourceDriver, _, m, err := setup(context.Background(), db, migrations, slog.Make())
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
	_, _, m, err := setup(context.Background(), db, migrations, slog.Make())
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
