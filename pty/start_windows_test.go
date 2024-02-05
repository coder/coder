//go:build windows
// +build windows

package pty_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/pty/ptytest"
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
		ptty, ps := ptytest.Start(t, pty.Command("cmd.exe", "/c", "echo", "test"))
		ptty.ExpectMatch("test")
		err := ps.Wait()
		require.NoError(t, err)
		err = ptty.Close()
		require.NoError(t, err)
	})
	t.Run("Resize", func(t *testing.T) {
		t.Parallel()
		ptty, _ := ptytest.Start(t, pty.Command("cmd.exe"))
		err := ptty.Resize(100, 50)
		require.NoError(t, err)
		err = ptty.Close()
		require.NoError(t, err)
	})
	t.Run("Kill", func(t *testing.T) {
		t.Parallel()
		ptty, ps := ptytest.Start(t, pty.Command("cmd.exe"))
		err := ps.Kill()
		assert.NoError(t, err)
		err = ps.Wait()
		var exitErr *exec.ExitError
		require.True(t, xerrors.As(err, &exitErr))
		assert.NotEqual(t, 0, exitErr.ExitCode())
		err = ptty.Close()
		require.NoError(t, err)
	})
	t.Run("Interrupt", func(t *testing.T) {
		t.Parallel()
		ptty, ps := ptytest.Start(t, pty.Command("cmd.exe"))
		err := ps.Signal(os.Interrupt) // Actually does kill.
		assert.NoError(t, err)
		err = ps.Wait()
		var exitErr *exec.ExitError
		require.True(t, xerrors.As(err, &exitErr))
		assert.NotEqual(t, 0, exitErr.ExitCode())
		err = ptty.Close()
		require.NoError(t, err)
	})
}

// these constants/vars are used by Test_Start_copy

const cmdEcho = "cmd.exe"

var argEcho = []string{"/c", "echo", "test"}

// these constants/vars are used by Test_Start_truncate

const (
	countEnd = 1000
	cmdCount = "cmd.exe"
)

var argCount = []string{"/c", fmt.Sprintf("for /L %%n in (1,1,%d) do @echo %%n", countEnd)}

// these constants/vars are used by Test_Start_cancel_context

const cmdSleep = "cmd.exe"

var argSleep = []string{"/c", "timeout", "/t", "30"}
