package testutil

import (
	"os"
	"path/filepath"
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

	t.Cleanup(func() {
		// The test might have created created and it may have already removed it,
		// so we ignore the error.
		_ = os.Remove(name)
	})

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

// TempDirResolved returns t.TempDir() with symlinks resolved via
// filepath.EvalSymlinks. Tests that compare paths against values
// processed by EvalSymlinks (directly or indirectly) should use
// this helper so the comparison works on macOS, where the default
// temp dir lives under /var which is a symlink to /private/var.
//
// If EvalSymlinks errors (for example on Windows where the temp
// path may not resolve cleanly), the raw t.TempDir() result is
// returned. This matches the lenient behavior already used in
// existing tests.
func TempDirResolved(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return resolved
	}
	return dir
}
