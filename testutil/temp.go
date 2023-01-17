package testutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// CreateTemp is a convenience function for creating a temporary file, like
// os.CreateTemp, but it also registers a cleanup function to close and remove
// the file.
func CreateTemp(t *testing.T, dir, pattern string) *os.File {
	t.Helper()

	if dir == "" {
		dir = t.TempDir()
	}
	f, err := os.CreateTemp(dir, pattern)
	require.NoError(t, err, "create temp file")
	t.Cleanup(func() {
		err = f.Close()
		if err != nil {
			t.Logf("CreateTemp: Cleanup: close failed for %q: %v", f.Name(), err)
		}
		err = os.Remove(f.Name())
		if err != nil {
			t.Logf("CreateTemp: Cleanup: remove failed for %q: %v", f.Name(), err)
		}
	})
	return f
}
