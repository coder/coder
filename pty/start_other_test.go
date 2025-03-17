//go:build !windows

package pty_test

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/v2/pty"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.GoleakOptions...)
}

func TestStart(t *testing.T) {
	t.Parallel()
	t.Run("Echo", func(t *testing.T) {
		t.Parallel()
		pty, ps := ptytest.Start(t, pty.Command("echo", "test"))

		pty.ExpectMatch("test")
		err := ps.Wait()
		require.NoError(t, err)
		err = pty.Close()
		require.NoError(t, err)
	})

	t.Run("Kill", func(t *testing.T) {
		t.Parallel()
		pty, ps := ptytest.Start(t, pty.Command("sleep", "30"))
		err := ps.Kill()
		assert.NoError(t, err)
		err = ps.Wait()
		var exitErr *exec.ExitError
		require.True(t, errors.As(err, &exitErr))
		assert.NotEqual(t, 0, exitErr.ExitCode())
		err = pty.Close()
		require.NoError(t, err)
	})

	t.Run("Interrupt", func(t *testing.T) {
		t.Parallel()
		pty, ps := ptytest.Start(t, pty.Command("sleep", "30"))
		err := ps.Signal(os.Interrupt)
		assert.NoError(t, err)
		err = ps.Wait()
		var exitErr *exec.ExitError
		require.True(t, errors.As(err, &exitErr))
		assert.NotEqual(t, 0, exitErr.ExitCode())
		err = pty.Close()
		require.NoError(t, err)
	})

	t.Run("SSH_TTY", func(t *testing.T) {
		t.Parallel()
		opts := pty.WithPTYOption(pty.WithSSHRequest(ssh.Pty{
			Window: ssh.Window{
				Width:  80,
				Height: 24,
			},
		}))
		pty, ps := ptytest.Start(t, pty.Command(`/bin/sh`, `-c`, `env | grep SSH_TTY`), opts)
		pty.ExpectMatch("SSH_TTY=/dev/")
		err := ps.Wait()
		require.NoError(t, err)
		err = pty.Close()
		require.NoError(t, err)
	})
}

// these constants/vars are used by Test_Start_copy

const cmdEcho = "echo"

var argEcho = []string{"test"}

// these constants/vars are used by Test_Start_truncate

const (
	countEnd = 1000
	cmdCount = "sh"
)

var argCount = []string{"-c", `
i=0
while [ $i -ne 1000 ]
do
        i=$(($i+1))
        echo "$i"
done
`}

// these constants/vars are used by Test_Start_cancel_context

const cmdSleep = "sleep"

var argSleep = []string{"30"}
