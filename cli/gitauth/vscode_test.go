package gitauth_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cli/gitauth"
)

func TestOverrideVSCodeConfigs(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	configPaths := []string{
		filepath.Join(xdg.DataHome, "code-server", "Machine", "settings.json"),
		filepath.Join(home, ".vscode-server", "data", "Machine", "settings.json"),
	}
	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		fs := afero.NewMemMapFs()
		err := gitauth.OverrideVSCodeConfigs(fs)
		require.NoError(t, err)
		for _, configPath := range configPaths {
			data, err := afero.ReadFile(fs, configPath)
			require.NoError(t, err)
			mapping := map[string]interface{}{}
			err = json.Unmarshal(data, &mapping)
			require.NoError(t, err)
			require.Equal(t, false, mapping["git.useIntegratedAskPass"])
			require.Equal(t, false, mapping["github.gitAuthentication"])
		}
	})
	t.Run("Append", func(t *testing.T) {
		t.Parallel()
		fs := afero.NewMemMapFs()
		mapping := map[string]interface{}{
			"hotdogs": "something",
		}
		data, err := json.Marshal(mapping)
		require.NoError(t, err)
		for _, configPath := range configPaths {
			err = afero.WriteFile(fs, configPath, data, 0o600)
			require.NoError(t, err)
		}
		err = gitauth.OverrideVSCodeConfigs(fs)
		require.NoError(t, err)
		for _, configPath := range configPaths {
			data, err := afero.ReadFile(fs, configPath)
			require.NoError(t, err)
			mapping := map[string]interface{}{}
			err = json.Unmarshal(data, &mapping)
			require.NoError(t, err)
			require.Equal(t, false, mapping["git.useIntegratedAskPass"])
			require.Equal(t, false, mapping["github.gitAuthentication"])
			require.Equal(t, "something", mapping["hotdogs"])
		}
	})
}
