//go:build !slim

package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"golang.org/x/xerrors"
)

// BuiltinPostgresConfig selects the binaries matching the existing data directory.
// A missing data directory is safe to initialize; an existing directory without a
// supported PG_VERSION must not be passed to embedded-postgres, which would remove it.
func BuiltinPostgresConfig(postgresPath string) (embeddedpostgres.PostgresVersion, string, bool, error) {
	dataPath := filepath.Join(postgresPath, "data")
	_, err := os.Stat(dataPath)
	if errors.Is(err, os.ErrNotExist) {
		return embeddedpostgres.V18, filepath.Join(postgresPath, "bin-"+string(embeddedpostgres.V18)), true, nil
	}
	if err != nil {
		return "", "", false, xerrors.Errorf("inspect built-in PostgreSQL data directory: %w", err)
	}

	version, err := os.ReadFile(filepath.Join(dataPath, "PG_VERSION"))
	if err != nil {
		advice := "Check that Coder can read PG_VERSION and the data directory before retrying."
		if errors.Is(err, os.ErrNotExist) {
			advice = "Restore the data directory from a backup, or if this is a new deployment, move it aside and restart Coder. Do not create PG_VERSION manually."
		}
		return "", "", false, xerrors.Errorf("read built-in PostgreSQL version in %s: %w; data was not changed. %s", dataPath, err, advice)
	}
	marker := strings.TrimSuffix(string(version), "\n")
	switch marker {
	case "13":
		return embeddedpostgres.V13, filepath.Join(postgresPath, "bin"), false, nil
	case "18":
		return embeddedpostgres.V18, filepath.Join(postgresPath, "bin-"+string(embeddedpostgres.V18)), false, nil
	default:
		return "", "", false, xerrors.Errorf("unsupported built-in PostgreSQL version %q in %s; data was not changed", marker, filepath.Join(dataPath, "PG_VERSION"))
	}
}
