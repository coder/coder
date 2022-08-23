//go:build !windows

package pty_test

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"go.uber.org/goleak"

	"github.com/coder/coder/pty/ptytest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestStart(t *testing.T) {
	t.Parallel()
	t.Run("Echo", func(t *testing.T) {
		t.Parallel()
		pty, ps := ptytest.Start(t, exec.Command("echo", "test"))
		pty.ExpectMatch("test")
		err := ps.Wait()
		require.NoError(t, err)
	})

	t.Run("Kill", func(t *testing.T) {
		t.Parallel()
		_, ps := ptytest.Start(t, exec.Command("sleep", "30"))
		err := ps.Kill()
		assert.NoError(t, err)
		err = ps.Wait()
		var exitErr *exec.ExitError
		require.True(t, xerrors.As(err, &exitErr))
		assert.NotEqual(t, 0, exitErr.ExitCode())
	})

	t.Run("SSH_PTY", func(t *testing.T) {
		t.Parallel()
		pty, ps := ptytest.Start(t, exec.Command("env"))
		pty.ExpectMatch("SSH_PTY=/dev/")
		err := ps.Wait()
		require.NoError(t, err)
	})
}
