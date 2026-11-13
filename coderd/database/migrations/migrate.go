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
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/quartz"
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

// setup prepares a migration run over migs, or the embedded migrations when
// migs is nil. A nil reporter reports nothing.
func setup(db *sql.DB, migs fs.FS, r *reporter) (source.Driver, *migrate.Migrate, error) {
	if migs == nil {
		migs = migrations
	}
	ctx := context.Background()
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

	if r != nil {
		r.source = sourceDriver
	}
	dbDriver := &pgTxnDriver{ctx: context.Background(), db: db, reporter: r, phase: phaseBatch}
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

// Option configures Up.
type Option func(*upOptions)

type upOptions struct {
	fs                fs.FS
	logger            *slog.Logger
	appName           string
	clock             quartz.Clock
	heartbeatInterval time.Duration
	observerDB        *sql.DB
}

func newUpOptions(opts ...Option) upOptions {
	var o upOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithFS runs the migrations in fsys instead of Coder's embedded migrations.
func WithFS(fsys fs.FS) Option {
	return func(o *upOptions) {
		o.fs = fsys
	}
}

// WithLogger writes the progress of the migrations to logger: waits for the
// migration lock, each migration as it is applied, and a heartbeat every 10
// seconds while a wait or a migration lasts. The migration connection is
// named coder-migrate/<version> in pg_stat_activity. Without it, migrations
// report nothing.
func WithLogger(logger slog.Logger) Option {
	return func(o *upOptions) {
		o.logger = &logger
	}
}

// Up runs SQL migrations to ensure the database schema is up-to-date. By
// default it runs Coder's embedded migrations and reports nothing; WithFS and
// WithLogger change that. Reporting never changes how the migrations run,
// lock or commit.
func Up(db *sql.DB, opts ...Option) (retErr error) {
	o := newUpOptions(opts...)
	r := newReporter(db, o)
	_, m, err := setup(db, o.fs, r)
	if err != nil {
		r.close()
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
	if errors.Is(err, migrate.ErrNoChange) {
		// It's OK if no changes happened!
		err = nil
	}

	_, err = r.finish(err)
	return err
}

// Down runs all down SQL migrations.
func Down(db *sql.DB) error {
	_, m, err := setup(db, migrations, nil)
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
	sourceDriver, m, err := setup(db, migrations, nil)
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
	_, m, err := setup(db, migrations, nil)
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
