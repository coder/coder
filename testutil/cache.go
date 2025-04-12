package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// PersistentCacheDir returns a path to a directory
// that will be cached between test runs in Github Actions.
func PersistentCacheDir(t *testing.T) string {
	t.Helper()

	// We don't use os.UserCacheDir() because the path it
	// returns is different on different operating systems.
	// This would make it harder to specify which cache dir to use
	// in Github Actions.
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	dir := filepath.Join(home, ".cache", "coderv2-test")

	return dir
}
