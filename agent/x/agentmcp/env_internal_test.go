package agentmcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func TestFilterPluginEnv(t *testing.T) {
	t.Parallel()

	in := []string{
		"PATH=/usr/bin",
		"HOME=/home/u",
		"CODER_AGENT_TOKEN=secret",
		"CODER_AGENT_URL=https://coder.example.com",
		"LC_ALL=C.UTF-8",
		"LC_TIME=en_US.UTF-8",
		"XDG_CONFIG_HOME=/home/u/.config",
		"LD_PRELOAD=/evil.so",
		"GITHUB_TOKEN=ghp",
		"https_proxy=http://proxy:3128",
		"NO_PROXY=localhost",
		"SSL_CERT_FILE=/etc/ssl/ca.pem",
		"NODE_EXTRA_CA_CERTS=/etc/ssl/ca.pem",
		"TMPDIR=/tmp",
		"NOEQUALS",
		"LCX=not-a-prefix-match",
		"XDGSOMETHING=1",
	}
	want := []string{
		"PATH=/usr/bin",
		"HOME=/home/u",
		"LC_ALL=C.UTF-8",
		"LC_TIME=en_US.UTF-8",
		"XDG_CONFIG_HOME=/home/u/.config",
		"https_proxy=http://proxy:3128",
		"NO_PROXY=localhost",
		"SSL_CERT_FILE=/etc/ssl/ca.pem",
		"NODE_EXTRA_CA_CERTS=/etc/ssl/ca.pem",
		"TMPDIR=/tmp",
	}
	assert.Equal(t, want, filterPluginEnv(in))
}

func TestBuildPluginEnv(t *testing.T) {
	t.Parallel()

	scope := PluginScope{Name: "p", Root: "/plugins/p", DataDir: "/data/p"}

	t.Run("HomeFallbackAndPlaceholders", func(t *testing.T) {
		t.Parallel()
		m := &Manager{envInfo: &fakeEnvInfo{
			environ: []string{"PATH=/usr/bin", "CODER_AGENT_TOKEN=secret", "LC_ALL=C"},
			home:    "/home/fallback",
		}}
		got := m.buildPluginEnv(scope, map[string]string{"EXTRA": "1", "PATH": "/plugin/bin"})
		assert.Equal(t, []string{
			"PATH=/plugin/bin",
			"LC_ALL=C",
			"HOME=/home/fallback",
			"PLUGIN_DATA=/data/p",
			"PLUGIN_ROOT=/plugins/p",
			"EXTRA=1",
		}, got)
	})

	t.Run("HomePresentIsKept", func(t *testing.T) {
		t.Parallel()
		m := &Manager{envInfo: &fakeEnvInfo{
			environ: []string{"HOME=/home/real"},
			home:    "/home/fallback",
		}}
		got := m.buildPluginEnv(scope, nil)
		assert.Equal(t, []string{
			"HOME=/home/real",
			"PLUGIN_DATA=/data/p",
			"PLUGIN_ROOT=/plugins/p",
		}, got)
	})

	t.Run("UpdateEnvNotConsulted", func(t *testing.T) {
		t.Parallel()
		m := &Manager{
			envInfo: &fakeEnvInfo{environ: []string{"PATH=/usr/bin"}, home: "/home/u"},
			updateEnv: func([]string) ([]string, error) {
				t.Fatal("updateEnv must not be called for plugin servers")
				return nil, nil
			},
		}
		got := m.buildPluginEnv(scope, nil)
		require.Contains(t, got, "PATH=/usr/bin")
		assert.NotContains(t, got, "CODER_AGENT_TOKEN=secret")
	})
}
