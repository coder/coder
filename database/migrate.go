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

// Migrate runs SQL migrations to ensure the database schema is up-to-date.
func Migrate(db *sql.DB) error {
	sourceDriver, err := iofs.New(migrations, "migrations")
	if err != nil {
		return xerrors.Errorf("create iofs: %w", err)
	}
	dbDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return xerrors.Errorf("wrap postgres connection: %w", err)
	}
	m, err := migrate.NewWithInstance("", sourceDriver, "", dbDriver)
	if err != nil {
		return xerrors.Errorf("migrate: %w", err)
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
