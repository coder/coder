package migrations

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"golang.org/x/xerrors"
)

//go:embed *.sql
var migrations embed.FS

func setup(db *sql.DB) (source.Driver, *migrate.Migrate, error) {
	ctx := context.Background()
	sourceDriver, err := iofs.New(migrations, ".")
	if err != nil {
		return nil, nil, xerrors.Errorf("create iofs: %w", err)
	}

	// there is a postgres.WithInstance() method that takes the DB instance,
	// but, when you close the resulting Migrate, it closes the DB, which
	// we don't want.  Instead, create just a connection that will get closed
	// when migration is done.
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, nil, xerrors.Errorf("postgres connection: %w", err)
	}
	dbDriver, err := postgres.WithConnection(ctx, conn, &postgres.Config{})
	if err != nil {
		return nil, nil, xerrors.Errorf("wrap postgres connection: %w", err)
	}

	m, err := migrate.NewWithInstance("", sourceDriver, "", dbDriver)
	if err != nil {
		return nil, nil, xerrors.Errorf("new migrate instance: %w", err)
	}

	return sourceDriver, m, nil
}

// Up runs SQL migrations to ensure the database schema is up-to-date.
func Up(db *sql.DB) (retErr error) {
	_, m, err := setup(db)
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

// Down runs all down SQL migrations.
func Down(db *sql.DB) error {
	_, m, err := setup(db)
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
	sourceDriver, m, err := setup(db)
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
