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
		wantAdvice  string
	}{
		{name: "new", wantVersion: embeddedpostgres.V18, wantBin: "bin-" + string(embeddedpostgres.V18), wantFresh: true},
		{name: "existing 13", dataVersion: "13\n", wantVersion: embeddedpostgres.V13, wantBin: "bin"},
		{name: "existing 18", dataVersion: "18\n", wantVersion: embeddedpostgres.V18, wantBin: "bin-" + string(embeddedpostgres.V18)},
		{name: "unsupported", dataVersion: "16\n", wantErr: "unsupported built-in PostgreSQL version"},
		{name: "missing version", dataVersion: "", wantErr: "read built-in PostgreSQL version", wantAdvice: "Restore the data directory from a backup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pgDir := t.TempDir()
			dataDir := filepath.Join(pgDir, "data")
			if tc.name != "new" {
				require.NoError(t, os.Mkdir(dataDir, 0o700))
				if tc.dataVersion != "" {
					require.NoError(t, os.WriteFile(filepath.Join(dataDir, "PG_VERSION"), []byte(tc.dataVersion), 0o600))
				}
				if tc.wantErr != "" {
					require.NoError(t, os.WriteFile(filepath.Join(dataDir, "keep"), []byte("user data"), 0o600))
				}
			}

			version, binDir, fresh, err := config.BuiltinPostgresConfig(pgDir)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				if tc.wantAdvice != "" {
					require.ErrorContains(t, err, tc.wantAdvice)
					require.ErrorContains(t, err, "move it aside and restart Coder")
					require.ErrorContains(t, err, "Do not create PG_VERSION manually")
				}
				contents, err := os.ReadFile(filepath.Join(dataDir, "keep"))
				require.NoError(t, err)
				require.Equal(t, "user data", string(contents))
				if tc.dataVersion != "" {
					version, err := os.ReadFile(filepath.Join(dataDir, "PG_VERSION"))
					require.NoError(t, err)
					require.Equal(t, tc.dataVersion, string(version))
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantVersion, version)
			require.Equal(t, filepath.Join(pgDir, tc.wantBin), binDir)
			require.Equal(t, tc.wantFresh, fresh)
		})
	}
}
