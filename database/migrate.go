package database

import (
	"context"
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
func Migrate(ctx context.Context, dbName string, db *sql.DB) error {
	sourceDriver, err := iofs.New(migrations, "migrations")
	if err != nil {
		return xerrors.Errorf("create iofs: %w", err)
	}
	dbDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return xerrors.Errorf("wrap postgres connection: %w", err)
	}
	m, err := migrate.NewWithInstance("", sourceDriver, dbName, dbDriver)
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
	srcErr, dbErr := m.Close()
	if srcErr != nil {
		return xerrors.Errorf("close source: %w", err)
	}
	if dbErr != nil {
		return xerrors.Errorf("close database: %w", err)
	}
	return nil
}
