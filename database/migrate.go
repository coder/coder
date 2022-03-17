package database

import (
	"database/sql"
	"embed"
	"errors"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"golang.org/x/xerrors"
)

//go:embed migrations/*.sql
var migrations embed.FS

func migrateSetup(db *sql.DB) (*migrate.Migrate, error) {
	sourceDriver, err := iofs.New(migrations, "migrations")
	if err != nil {
		return nil, xerrors.Errorf("create iofs: %w", err)
	}

	dbDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, xerrors.Errorf("wrap postgres connection: %w", err)
	}

	m, err := migrate.NewWithInstance("", sourceDriver, "", dbDriver)
	if err != nil {
		return nil, xerrors.Errorf("new migrate instance: %w", err)
	}

	return m, nil
}

// MigrateUp runs SQL migrations to ensure the database schema is up-to-date.
func MigrateUp(db *sql.DB) error {
	m, err := migrateSetup(db)
	if err != nil {
		return xerrors.Errorf("migrate setup: %w", err)
	}

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

// MigrateDown runs all down SQL migrations.
func MigrateDown(db *sql.DB) error {
	m, err := migrateSetup(db)
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
