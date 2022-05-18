package cliui_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/pty"
	"github.com/coder/coder/pty/ptytest"
)

func TestPrompt(t *testing.T) {
	t.Parallel()
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ptty := ptytest.New(t)
		msgChan := make(chan string)
		go func() {
			resp, err := newPrompt(ptty, cliui.PromptOptions{
				Text: "Example",
			})
			require.NoError(t, err)
			msgChan <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine("hello")
		require.Equal(t, "hello", <-msgChan)
	})

	t.Run("Confirm", func(t *testing.T) {
		t.Parallel()
		ptty := ptytest.New(t)
		doneChan := make(chan string)
		go func() {
			resp, err := newPrompt(ptty, cliui.PromptOptions{
				Text:      "Example",
				IsConfirm: true,
			})
			require.NoError(t, err)
			doneChan <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine("yes")
		require.Equal(t, "yes", <-doneChan)
	})

	t.Run("JSON", func(t *testing.T) {
		t.Parallel()
		ptty := ptytest.New(t)
		doneChan := make(chan string)
		go func() {
			resp, err := newPrompt(ptty, cliui.PromptOptions{
				Text: "Example",
			})
			require.NoError(t, err)
			doneChan <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine("{}")
		require.Equal(t, "{}", <-doneChan)
	})

	t.Run("BadJSON", func(t *testing.T) {
		t.Parallel()
		ptty := ptytest.New(t)
		doneChan := make(chan string)
		go func() {
			resp, err := newPrompt(ptty, cliui.PromptOptions{
				Text: "Example",
			})
			require.NoError(t, err)
			doneChan <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine("{a")
		require.Equal(t, "{a", <-doneChan)
	})

	t.Run("MultilineJSON", func(t *testing.T) {
		t.Parallel()
		ptty := ptytest.New(t)
		doneChan := make(chan string)
		go func() {
			resp, err := newPrompt(ptty, cliui.PromptOptions{
				Text: "Example",
			})
			require.NoError(t, err)
			doneChan <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine(`{
"test": "wow"
}`)
		require.Equal(t, `{"test":"wow"}`, <-doneChan)
	})
}

func newPrompt(ptty *ptytest.PTY, opts cliui.PromptOptions) (string, error) {
	value := ""
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			value, err = cliui.Prompt(cmd, opts)
			return err
		},
	}
	cmd.SetOutput(ptty.Output())
	cmd.SetIn(ptty.Input())
	return value, cmd.ExecuteContext(context.Background())
}

func TestPasswordTerminalState(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS") == "1" {
		passwordHelper()
		return
	}
	t.Parallel()

	ptty := ptytest.New(t)
	ptyWithFlags, ok := ptty.PTY.(pty.WithFlags)
	if !ok {
		t.Skip("unable to check PTY local echo on this platform")
	}

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
	time.Sleep(100 * time.Millisecond) // wait for child process to turn off echo and start reading input

	echo, err := ptyWithFlags.EchoEnabled()
	require.NoError(t, err)
	require.False(t, echo, "echo is on while reading password")

	err = process.Signal(os.Interrupt)
	require.NoError(t, err)
	_, err = process.Wait()
	require.NoError(t, err)

	echo, err = ptyWithFlags.EchoEnabled()
	require.NoError(t, err)
	require.True(t, echo, "echo is off after reading password")
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
