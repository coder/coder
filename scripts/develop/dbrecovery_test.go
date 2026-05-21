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
