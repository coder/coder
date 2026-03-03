package testutil

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TempDirUnixSocket returns a temporary directory that can safely hold unix
// sockets (probably).
//
// During tests on darwin we hit the max path length limit for unix sockets
// pretty easily in the default location, so this function uses /tmp instead to
// get shorter paths.
//
// On Linux, we also hit this limit on GitHub Actions runners where TMPDIR is
// set to a long path like /home/runner/work/_temp/go-tmp/.
func TempDirUnixSocket(t *testing.T) string {
	t.Helper()
	// Windows doesn't have the same unix socket path length limits,
	// and callers of this function are generally gated to !windows.
	if runtime.GOOS == "windows" {
		return t.TempDir()
	}

	testName := strings.ReplaceAll(t.Name(), "/", "_")
	dir, err := os.MkdirTemp("/tmp", testName)
	require.NoError(t, err, "create temp dir for unix socket test")

	t.Cleanup(func() {
		err := os.RemoveAll(dir)
		assert.NoError(t, err, "remove temp dir", dir)
	})
	return dir
}
