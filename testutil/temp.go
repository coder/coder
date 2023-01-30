package testutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TempFile returns the name of a temporary file that does not exist.
func TempFile(t *testing.T, dir, pattern string) string {
	t.Helper()

	if dir == "" {
		dir = t.TempDir()
	}
	f, err := os.CreateTemp(dir, pattern)
	require.NoError(t, err, "create temp file")
	name := f.Name()
	err = f.Close()
	require.NoError(t, err, "close temp file")
	err = os.Remove(name)
	require.NoError(t, err, "remove temp file")

	return name
}

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
		_ = f.Close()
		err = os.Remove(f.Name())
		if err != nil {
			t.Logf("CreateTemp: Cleanup: remove failed for %q: %v", f.Name(), err)
		}
	})
	return f
}
