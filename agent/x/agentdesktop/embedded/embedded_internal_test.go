package embedded

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// setBinary swaps the embedded bytes for the duration of a test. Tests using
// it must not run in parallel.
func setBinary(t *testing.T, b []byte) {
	t.Helper()
	old := binary
	binary = b
	t.Cleanup(func() { binary = old })
}

//nolint:paralleltest // setBinary mutates package state.
func TestInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable bits are not reported on Windows")
	}
	body := []byte("#!/bin/sh\necho portabledesktop\n")
	setBinary(t, body)
	require.True(t, Available())
	require.Len(t, SHA256(), 64)

	cacheDir := t.TempDir()
	path, err := Install(cacheDir)
	require.NoError(t, err)
	require.Equal(t, InstallPath(cacheDir), path)
	require.Contains(t, path, Version+"-"+SHA256()[:12])

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&0o111, "installed binary must be executable")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, body, got)

	// A second install reuses the existing file.
	before := info.ModTime()
	path2, err := Install(cacheDir)
	require.NoError(t, err)
	require.Equal(t, path, path2)
	info2, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, before, info2.ModTime())

	// A corrupted install is replaced.
	require.NoError(t, os.WriteFile(path, []byte("corrupt"), 0o600))
	_, err = Install(cacheDir)
	require.NoError(t, err)
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, body, got)

	// No temp files are left behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

//nolint:paralleltest // setBinary mutates package state.
func TestInstallNotAvailable(t *testing.T) {
	setBinary(t, nil)
	require.False(t, Available())
	require.Empty(t, SHA256())
	_, err := Install(t.TempDir())
	require.ErrorIs(t, err, ErrNotAvailable)
}

func TestDefaultCacheDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "xdg"))
	dir, err := DefaultCacheDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(os.Getenv("XDG_CACHE_HOME"), "coder"), dir)
}
