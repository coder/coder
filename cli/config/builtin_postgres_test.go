package config_test

import (
	"os"
	"path/filepath"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/config"
)

func TestBuiltinPostgresConfig(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		dataVersion string
		wantVersion embeddedpostgres.PostgresVersion
		wantBin     string
		wantFresh   bool
		wantErr     string
	}{
		{name: "new", wantVersion: "18.6.0", wantBin: "bin-18.6.0", wantFresh: true},
		{name: "existing 13", dataVersion: "13\n", wantVersion: embeddedpostgres.V13, wantBin: "bin"},
		{name: "existing 18", dataVersion: "18\n", wantVersion: "18.6.0", wantBin: "bin-18.6.0"},
		{name: "unsupported", dataVersion: "16\n", wantErr: "unsupported built-in PostgreSQL version"},
		{name: "missing version", dataVersion: "", wantErr: "read built-in PostgreSQL version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pgDir := t.TempDir()
			if tc.name != "new" {
				dataDir := filepath.Join(pgDir, "data")
				require.NoError(t, os.Mkdir(dataDir, 0o700))
				if tc.dataVersion != "" {
					require.NoError(t, os.WriteFile(filepath.Join(dataDir, "PG_VERSION"), []byte(tc.dataVersion), 0o600))
				}
			}

			version, binDir, fresh, err := config.BuiltinPostgresConfig(pgDir)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.DirExists(t, filepath.Join(pgDir, "data"))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantVersion, version)
			require.Equal(t, filepath.Join(pgDir, tc.wantBin), binDir)
			require.Equal(t, tc.wantFresh, fresh)
		})
	}
}
