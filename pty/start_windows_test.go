//go:build windows
// +build windows

package pty_test

import (
	"os/exec"
	"testing"

	"github.com/coder/coder/pty/ptytest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestStart(t *testing.T) {
	t.Parallel()
	t.Run("Echo", func(t *testing.T) {
		t.Parallel()
		pty, ps := ptytest.Start(t, exec.Command("cmd.exe", "/c", "echo", "test"))
		pty.ExpectMatch("test")
		err := ps.Wait()
		require.NoError(t, err)
	})
	t.Run("Resize", func(t *testing.T) {
		t.Parallel()
		pty := ptytest.Start(t, exec.Command("cmd.exe"))
		err := pty.Resize(100, 50)
		require.NoError(t, err)
	})
	t.Run("Kill", func(t *testing.T) {
		t.Parallel()
		pty := ptytest.Start(t, exec.Command("cmd.exe"))
		err := ps.Kill()
		assert.NoError(t, err)
		err = ps.Wait()
		var exitErr *exec.ExitError
		require.True(t, xerrors.As(err, &exitErr))
		assert.Equal(t, -1, exitErr.ExitCode())
	})
}
