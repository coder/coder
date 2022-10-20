package config_test

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/config"
)

func TestServer(t *testing.T) {
	t.Parallel()
	t.Run("WritesDefault", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "server.yaml")
		_, err := config.ParseServer(&cobra.Command{}, &url.URL{}, path)
		require.NoError(t, err)
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Greater(t, len(data), 0)
	})
	t.Run("Filled", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "server.yaml")
		err := os.WriteFile(path, []byte(`
gitauth:
- type: github
  client_id: xxx
  client_secret: xxx
`), 0600)
		require.NoError(t, err)
		cfg, err := config.ParseServer(&cobra.Command{}, &url.URL{}, path)
		require.NoError(t, err)
		require.Len(t, cfg.GitAuth, 1)
	})
}
