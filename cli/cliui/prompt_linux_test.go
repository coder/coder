//go:build linux

package cliui_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/pty/ptytest"
)

func TestPasswordTerminalState(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS") == "1" {
		passwordHelper()
		return
	}
	t.Parallel()

	ptty := ptytest.New(t)

	cmd := exec.Command(os.Args[0], "-test.run=TestPasswordTerminalState") //nolint:gosec
	cmd.Env = append(os.Environ(), "TEST_SUBPROCESS=1")
	// connect the child process's stdio to the PTY directly, not via a pipe
	cmd.Stdin = ptty.Input().Reader
	cmd.Stdout = ptty.Output().Writer
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	require.NoError(t, err)
	process := cmd.Process
	defer process.Kill()

	ptty.ExpectMatch("Password: ")
	time.Sleep(10 * time.Millisecond) // wait for child process to turn off echo and start reading input

	termios, err := unix.IoctlGetTermios(int(ptty.Input().Reader.Fd()), unix.TCGETS)
	require.NoError(t, err)
	require.Zero(t, termios.Lflag&unix.ECHO, "echo is on while reading password")

	err = process.Signal(os.Interrupt)
	require.NoError(t, err)
	_, err = process.Wait()
	require.NoError(t, err)

	termios, err = unix.IoctlGetTermios(int(ptty.Input().Reader.Fd()), unix.TCGETS)
	require.NoError(t, err)
	require.NotZero(t, termios.Lflag&unix.ECHO, "echo is off after reading password")
}

func passwordHelper() {
	cmd := &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			cliui.Prompt(cmd, cliui.PromptOptions{
				Text:   "Password:",
				Secret: true,
			})
		},
	}
	cmd.ExecuteContext(context.Background())
}
