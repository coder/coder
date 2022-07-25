package cliui_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
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
			}, nil)
			assert.NoError(t, err)
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
			}, nil)
			assert.NoError(t, err)
			doneChan <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine("yes")
		require.Equal(t, "yes", <-doneChan)
	})

	t.Run("Skip", func(t *testing.T) {
		t.Parallel()
		ptty := ptytest.New(t)
		var buf bytes.Buffer

		// Copy all data written out to a buffer. When we close the ptty, we can
		// no longer read from the ptty.Output(), but we can read what was
		// written to the buffer.
		dataRead, doneReading := context.WithTimeout(context.Background(), time.Second*2)
		go func() {
			// This will throw an error sometimes. The underlying ptty
			// has its own cleanup routines in t.Cleanup. Instead of
			// trying to control the close perfectly, just let the ptty
			// double close. This error isn't important, we just
			// want to know the ptty is done sending output.
			_, _ = io.Copy(&buf, ptty.Output())
			doneReading()
		}()

		doneChan := make(chan string)
		go func() {
			resp, err := newPrompt(ptty, cliui.PromptOptions{
				Text:      "ShouldNotSeeThis",
				IsConfirm: true,
			}, func(cmd *cobra.Command) {
				cliui.AllowSkipPrompt(cmd)
				cmd.SetArgs([]string{"-y"})
			})
			assert.NoError(t, err)
			doneChan <- resp
		}()

		require.Equal(t, "yes", <-doneChan)
		// Close the reader to end the io.Copy
		require.NoError(t, ptty.Close(), "close eof reader")
		// Wait for the IO copy to finish
		<-dataRead.Done()
		// Timeout error means the output was hanging
		require.ErrorIs(t, dataRead.Err(), context.Canceled, "should be canceled")
		require.Len(t, buf.Bytes(), 0, "expect no output")
	})
	t.Run("JSON", func(t *testing.T) {
		t.Parallel()
		ptty := ptytest.New(t)
		doneChan := make(chan string)
		go func() {
			resp, err := newPrompt(ptty, cliui.PromptOptions{
				Text: "Example",
			}, nil)
			assert.NoError(t, err)
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
			}, nil)
			assert.NoError(t, err)
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
			}, nil)
			assert.NoError(t, err)
			doneChan <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine(`{
"test": "wow"
}`)
		require.Equal(t, `{"test":"wow"}`, <-doneChan)
	})
}

func newPrompt(ptty *ptytest.PTY, opts cliui.PromptOptions, cmdOpt func(cmd *cobra.Command)) (string, error) {
	value := ""
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			value, err = cliui.Prompt(cmd, opts)
			return err
		},
	}
	// Optionally modify the cmd
	if cmdOpt != nil {
		cmdOpt(cmd)
	}
	cmd.SetOut(ptty.Output())
	cmd.SetErr(ptty.Output())
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
	cmd.Stderr = ptty.Output().Writer
	err := cmd.Start()
	require.NoError(t, err)
	process := cmd.Process
	defer process.Kill()

	ptty.ExpectMatch("Password: ")

	require.Eventually(t, func() bool {
		echo, err := ptyWithFlags.EchoEnabled()
		return err == nil && !echo
	}, 5*time.Second, 50*time.Millisecond, "echo is on while reading password")

	err = process.Signal(os.Interrupt)
	require.NoError(t, err)
	_, err = process.Wait()
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		echo, err := ptyWithFlags.EchoEnabled()
		return err == nil && echo
	}, 5*time.Second, 50*time.Millisecond, "echo is off after reading password")
}

// nolint:unused
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
