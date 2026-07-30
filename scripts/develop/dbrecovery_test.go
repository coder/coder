//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrationFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for path, content := range map[string]string{
		"000561_recent.up.sql":                     "recent up",
		"000561_recent.down.sql":                   "recent down",
		"000001-000100/000042_archived.up.sql":     "archived up",
		"000001-000100/000042_archived.down.sql":   "archived down",
		"000401-000500/000500_archived.up.sql":     "boundary up",
		"testdata/fixtures/000042_fixture.up.sql":  "fixture",
		"testdata/full_dumps/v0.6.6/000001.up.sql": "dump",
	} {
		full := filepath.Join(dir, filepath.FromSlash(path))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	}

	files, err := migrationFiles(dir)
	require.NoError(t, err)

	require.Equal(t, map[string]string{
		"000561_recent.up.sql":   filepath.Join(dir, "000561_recent.up.sql"),
		"000561_recent.down.sql": filepath.Join(dir, "000561_recent.down.sql"),
		"000042_archived.up.sql": filepath.Join(dir, "000001-000100", "000042_archived.up.sql"),
		"000042_archived.down.sql": filepath.Join(dir, "000001-000100",
			"000042_archived.down.sql"),
		"000500_archived.up.sql": filepath.Join(dir, "000401-000500", "000500_archived.up.sql"),
	}, files)
}

func TestMigrationFilesMatchesRepository(t *testing.T) {
	t.Parallel()

	files, err := migrationFiles(filepath.Join("..", "..", "coderd", "database", "migrations"))
	require.NoError(t, err)

	// Version 1 is archived, so resolving it proves recovery no longer depends on
	// every migration sitting in the migrations root.
	require.Contains(t, files, "000001_base.up.sql")
	require.Equal(t, "000001-000100", filepath.Base(filepath.Dir(files["000001_base.up.sql"])))
}

func TestCleanStalePIDFile(t *testing.T) {
	t.Parallel()

	t.Run("NoPIDFile", func(t *testing.T) {
		t.Parallel()
		cleanStalePIDFile(t.TempDir())
	})

	t.Run("StalePID", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		pidFile := filepath.Join(dir, "postmaster.pid")
		require.NoError(t, os.WriteFile(pidFile, []byte("999999999\n"), 0o600))

		cleanStalePIDFile(dir)

		_, err := os.Stat(pidFile)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("RunningPID", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		pidFile := filepath.Join(dir, "postmaster.pid")
		require.NoError(t, os.WriteFile(pidFile,
			[]byte(strconv.Itoa(os.Getpid())+"\n"), 0o600))

		cleanStalePIDFile(dir)

		_, err := os.Stat(pidFile)
		assert.NoError(t, err, "should not remove PID file for running process")
	})
}
