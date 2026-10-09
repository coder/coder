package agentacp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type testEnv struct {
	usershell.SystemEnvInfo
	home string
}

func (e testEnv) HomeDir() (string, error) { return e.home, nil }

func newTestManager(t *testing.T) (manager *Manager, configsDirectory string) {
	t.Helper()
	home := t.TempDir()
	m := NewManager(context.Background(), Options{
		Logger:     slogtest.Make(t, nil),
		Clock:      quartz.NewMock(t),
		Filesystem: afero.NewOsFs(),
		EnvInfo:    testEnv{home: home},
	})
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	configDir := filepath.Join(home, ".coder", "acp")
	require.NoError(t, os.MkdirAll(configDir, 0o700))
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": "false"})
	require.NoError(t, m.Reload(testutil.Context(t, testutil.WaitLong)))
	require.Len(t, m.Catalog(), 1)
	require.Empty(t, m.Catalog()[0].Error)
	return m, configDir
}

func writeConfig(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
}
