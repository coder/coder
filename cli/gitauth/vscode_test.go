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
	t.Run("MergeWithExistingSettings", func(t *testing.T) {
		t.Parallel()
		fs := afero.NewMemMapFs()
		// Create existing settings with user preferences
		existingSettings := map[string]interface{}{
			"workbench.colorTheme": "Dracula",
			"editor.fontSize": 14,
			"editor.tabSize": 2,
			"files.autoSave": "onWindowChange",
		}
		data, err := json.MarshalIndent(existingSettings, "", "\t")
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
			// Verify Coder settings are applied
			require.Equal(t, false, mapping["git.useIntegratedAskPass"])
			require.Equal(t, false, mapping["github.gitAuthentication"])
			// Verify user settings are preserved
			require.Equal(t, "Dracula", mapping["workbench.colorTheme"])
			require.Equal(t, float64(14), mapping["editor.fontSize"])
			require.Equal(t, float64(2), mapping["editor.tabSize"])
			require.Equal(t, "onWindowChange", mapping["files.autoSave"])
			// Verify no duplication - should have exactly 6 settings
			require.Len(t, mapping, 6)
		}
	})

	t.Run("MergeWithExistingCoderSettings", func(t *testing.T) {
		t.Parallel()
		fs := afero.NewMemMapFs()
		// Create existing settings that include Coder-specific settings with different values
		existingSettings := map[string]interface{}{
			"workbench.colorTheme": "Dark+",
			"git.useIntegratedAskPass": true, // This should be overridden to false
			"github.gitAuthentication": true, // This should be overridden to false
			"editor.wordWrap": "on",
			"terminal.integrated.shell.linux": "/bin/bash",
		}
		data, err := json.MarshalIndent(existingSettings, "", "\t")
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
			// Verify Coder settings override existing values
			require.Equal(t, false, mapping["git.useIntegratedAskPass"])
			require.Equal(t, false, mapping["github.gitAuthentication"])
			// Verify user settings are preserved
			require.Equal(t, "Dark+", mapping["workbench.colorTheme"])
			require.Equal(t, "on", mapping["editor.wordWrap"])
			require.Equal(t, "/bin/bash", mapping["terminal.integrated.shell.linux"])
			// Verify no duplication - should have exactly 5 settings
			require.Len(t, mapping, 5)
		}
	})

	t.Run("ValidJSONOutput", func(t *testing.T) {
		t.Parallel()
		fs := afero.NewMemMapFs()
		// Test with complex existing settings to ensure valid JSON output
		existingSettings := map[string]interface{}{
			"workbench.colorCustomizations": map[string]interface{}{
				"editor.background": "#1e1e1e",
				"sideBar.background": "#252526",
			},
			"extensions.recommendations": []string{"ms-python.python", "golang.go"},
			"git.useIntegratedAskPass": true,
			"editor.rulers": []int{80, 120},
		}
		data, err := json.MarshalIndent(existingSettings, "", "\t")
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
			// Verify the output is valid JSON
			mapping := map[string]interface{}{}
			err = json.Unmarshal(data, &mapping)
			require.NoError(t, err, "Output should be valid JSON")
			// Verify complex structures are preserved
			colorCustomizations, ok := mapping["workbench.colorCustomizations"].(map[string]interface{})
			require.True(t, ok, "Complex objects should be preserved")
			require.Equal(t, "#1e1e1e", colorCustomizations["editor.background"])
			// Verify arrays are preserved
			recommendations, ok := mapping["extensions.recommendations"].([]interface{})
			require.True(t, ok, "Arrays should be preserved")
			require.Len(t, recommendations, 2)
			// Verify Coder settings are applied
			require.Equal(t, false, mapping["git.useIntegratedAskPass"])
			require.Equal(t, false, mapping["github.gitAuthentication"])
		}
	})
}
