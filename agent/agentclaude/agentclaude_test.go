package agentclaude

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestConfigureClaude(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		cfg := ClaudeConfig{
			ConfigPath:       "/.claude.json",
			ProjectDirectory: "/home/coder/projects/coder/coder",
			APIKey:           "test-api-key",
		}
		err := configureClaude(fs, cfg)
		require.NoError(t, err)

		jsonBytes, err := afero.ReadFile(fs, cfg.ConfigPath)
		require.NoError(t, err)

		require.Equal(t, `{
  "autoUpdaterStatus": "disabled",
  "bypassPermissionsModeAccepted": true,
  "hasCompletedOnboarding": true,
  "primaryApiKey": "test-api-key",
  "projects": {
    "/home/coder/projects/coder/coder": {
      "allowedTools": [],
      "hasCompletedProjectOnboarding": true,
      "hasTrustDialogAccepted": true,
      "mcpServers": {}
    }
  }
}`, string(jsonBytes))
	})

	t.Run("override existing config", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/.claude.json", []byte(`{
			"bypassPermissionsModeAccepted": false,
			"hasCompletedOnboarding": false,
			"primaryApiKey": "magic-api-key"
		}`), 0644)
		cfg := ClaudeConfig{
			ConfigPath:       "/.claude.json",
			ProjectDirectory: "/home/coder/projects/coder/coder",
			APIKey:           "test-api-key",
		}
		err := configureClaude(fs, cfg)
		require.NoError(t, err)

		jsonBytes, err := afero.ReadFile(fs, cfg.ConfigPath)
		require.NoError(t, err)

		require.Equal(t, `{
  "autoUpdaterStatus": "disabled",
  "bypassPermissionsModeAccepted": true,
  "hasCompletedOnboarding": true,
  "primaryApiKey": "test-api-key",
  "projects": {
    "/home/coder/projects/coder/coder": {
      "allowedTools": [],
      "hasCompletedProjectOnboarding": true,
      "hasTrustDialogAccepted": true,
      "mcpServers": {}
    }
  }
}`, string(jsonBytes))
	})
}
