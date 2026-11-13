package agentmcp_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/x/agentmcp"
)

func TestPluginDataDir(t *testing.T) {
	t.Parallel()

	home := filepath.FromSlash("/home/coder")
	root := filepath.FromSlash("/work/plugins/demo")

	dataDir := func(name, root string) string {
		t.Helper()
		dir, err := agentmcp.PluginDataDir(home, name, root)
		require.NoError(t, err)
		return dir
	}

	got := dataDir("demo", root)
	assert.Equal(t, filepath.Join(home, ".coder", "plugin-data"), filepath.Dir(got))
	assert.Regexp(t, `^demo-[0-9a-f]{12}$`, filepath.Base(got))
	if runtime.GOOS != "windows" {
		// The returned path is part of the on-disk layout.
		assert.Equal(t, "/home/coder/.coder/plugin-data/demo-0875721e7ad7", got)
	}

	assert.Equal(t, got, dataDir("demo", root+string(filepath.Separator)),
		"a trailing separator names the same root")
	assert.NotEqual(t, got, dataDir("demo", filepath.FromSlash("/work/other/demo")),
		"two roots with the same plugin name must not share a directory")

	for _, name := range []string{"", "../../.ssh", "a/b", "Demo"} {
		_, err := agentmcp.PluginDataDir(home, name, root)
		assert.Error(t, err, name)
	}
}
