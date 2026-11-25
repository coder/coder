package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

// ValidateBuiltinPostgresVersion refuses to start over existing data that this
// Coder release cannot read. Embedded-postgres otherwise removes that data.
func ValidateBuiltinPostgresVersion(postgresPath string) error {
	dataPath := filepath.Join(postgresPath, "data")
	_, err := os.Stat(dataPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return xerrors.Errorf("inspect built-in PostgreSQL data directory: %w", err)
	}

	versionPath := filepath.Join(dataPath, "PG_VERSION")
	contents, err := os.ReadFile(versionPath)
	if err != nil {
		advice := "Check that Coder can read the file and data directory before retrying."
		if errors.Is(err, os.ErrNotExist) {
			advice = "Restore the data directory from a backup, or if this is an unused new deployment, move it aside and restart. Do not create PG_VERSION manually."
		}
		return xerrors.Errorf("read built-in PostgreSQL version in %s: %w; data was not changed. %s", dataPath, err, advice)
	}

	version := strings.TrimSuffix(string(contents), "\n")
	if version != "13" {
		return xerrors.Errorf("unsupported built-in PostgreSQL version %q in %s; data was not changed", version, versionPath)
	}
	return nil
}
