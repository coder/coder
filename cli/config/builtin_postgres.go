package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"golang.org/x/xerrors"
)

const builtinPostgresVersion = embeddedpostgres.PostgresVersion("18.6.0")

// BuiltinPostgresConfig selects the binaries matching the existing data directory.
// A missing data directory is safe to initialize; an existing directory without a
// supported PG_VERSION must not be passed to embedded-postgres, which would remove it.
func BuiltinPostgresConfig(postgresPath string) (embeddedpostgres.PostgresVersion, string, bool, error) {
	dataPath := filepath.Join(postgresPath, "data")
	_, err := os.Stat(dataPath)
	if errors.Is(err, os.ErrNotExist) {
		return builtinPostgresVersion, filepath.Join(postgresPath, "bin-"+string(builtinPostgresVersion)), true, nil
	}
	if err != nil {
		return "", "", false, xerrors.Errorf("inspect built-in PostgreSQL data directory: %w", err)
	}

	version, err := os.ReadFile(filepath.Join(dataPath, "PG_VERSION"))
	if err != nil {
		return "", "", false, xerrors.Errorf("read built-in PostgreSQL version in %s: %w; data was not changed", dataPath, err)
	}
	switch strings.TrimSpace(string(version)) {
	case "13":
		return embeddedpostgres.V13, filepath.Join(postgresPath, "bin"), false, nil
	case "18":
		return builtinPostgresVersion, filepath.Join(postgresPath, "bin-"+string(builtinPostgresVersion)), false, nil
	default:
		return "", "", false, xerrors.Errorf("unsupported built-in PostgreSQL version %q in %s; data was not changed", strings.TrimSpace(string(version)), dataPath)
	}
}
