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
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/usershell"
	"github.com/coder/quartz"
)

type testEnv struct {
	usershell.SystemEnvInfo
	home string
}

func (e testEnv) HomeDir() (string, error) { return e.home, nil }

func newTestManager(t *testing.T, mode fakeHarnessMode) (manager *Manager, directory, configsDirectory string) {
	t.Helper()
	home, dir := t.TempDir(), t.TempDir()
	m := NewManager(context.Background(), Options{
		Logger:     slogtest.Make(t, nil),
		Clock:      quartz.NewMock(t),
		Execer:     agentexec.DefaultExecer,
		Filesystem: afero.NewOsFs(),
		EnvInfo:    testEnv{home: home},
		WorkingDir: func() string { return dir },
	})
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	configDir := filepath.Join(home, ".coder", "acp")
	require.NoError(t, os.MkdirAll(configDir, 0o700))
	writeConfig(t, filepath.Join(configDir, "fake.json"), map[string]any{"command": fakeCommand(t, mode)})
	require.NoError(t, m.Reload(context.Background()))
	require.Len(t, m.Catalog(), 1)
	require.Empty(t, m.Catalog()[0].Error)
	return m, dir, configDir
}

func writeConfig(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
}
