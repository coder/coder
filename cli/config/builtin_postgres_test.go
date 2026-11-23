package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/config"
)

func TestValidateBuiltinPostgresVersionAcceptsExisting13(t *testing.T) {
	t.Parallel()

	postgresPath := t.TempDir()
	dataPath := filepath.Join(postgresPath, "data")
	require.NoError(t, os.Mkdir(dataPath, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dataPath, "PG_VERSION"), []byte("13\n"), 0o600))
	require.NoError(t, config.ValidateBuiltinPostgresVersion(postgresPath))
}
