package agentmcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/agent/usershell"
)

// fakeEnvInfo is a usershell.EnvInfoer with a controlled environment
// and home directory.
type fakeEnvInfo struct {
	usershell.SystemEnvInfo
	environ []string
	home    string
}

func (f *fakeEnvInfo) Environ() []string { return append([]string(nil), f.environ...) }

func (f *fakeEnvInfo) HomeDir() (string, error) { return f.home, nil }

func TestBuildPluginEnv(t *testing.T) {
	t.Parallel()

	scope := PluginScope{Name: "p", Root: "/plugins/p", DataDir: "/data/p"}

	t.Run("InheritsEnrichedEnvironment", func(t *testing.T) {
		t.Parallel()
		m := &Manager{
			envInfo: &fakeEnvInfo{environ: []string{"PATH=/usr/bin", "CODER_AGENT_TOKEN=secret"}},
			updateEnv: func(env []string) ([]string, error) {
				return append(env, "FROM_UPDATE_ENV=1"), nil
			},
		}
		got := m.buildPluginEnv(context.Background(), scope, map[string]string{"EXTRA": "1", "PATH": "/plugin/bin"})
		assert.Equal(t, []string{
			"PATH=/plugin/bin",
			"CODER_AGENT_TOKEN=secret",
			"FROM_UPDATE_ENV=1",
			"PLUGIN_DATA=/data/p",
			"PLUGIN_ROOT=/plugins/p",
			"EXTRA=1",
		}, got)
	})

	t.Run("PluginVariablesReplaceAmbient", func(t *testing.T) {
		t.Parallel()
		m := &Manager{envInfo: &fakeEnvInfo{
			environ: []string{"PLUGIN_ROOT=/elsewhere", "PLUGIN_DATA=/elsewhere-data"},
		}}
		got := m.buildPluginEnv(context.Background(), scope, nil)
		assert.Equal(t, []string{
			"PLUGIN_ROOT=/plugins/p",
			"PLUGIN_DATA=/data/p",
		}, got)
	})
}
