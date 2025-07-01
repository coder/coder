package cliui_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/pty/ptytest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/serpent"
)

func TestPrompt(t *testing.T) {
	t.Parallel()
	t.Run("Success", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ptty := ptytest.New(t)
		msgChan := make(chan string)
		go func() {
			resp, err := newPrompt(ctx, ptty, cliui.PromptOptions{
				Text: "Example",
			}, nil)
			assert.NoError(t, err)
			msgChan <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine("hello")
		resp := testutil.TryReceive(ctx, t, msgChan)
		require.Equal(t, "hello", resp)
	})

	t.Run("Confirm", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ptty := ptytest.New(t)
		doneChan := make(chan string)
		go func() {
			resp, err := newPrompt(ctx, ptty, cliui.PromptOptions{
				Text:      "Example",
				IsConfirm: true,
			}, nil)
			assert.NoError(t, err)
			doneChan <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine("yes")
		resp := testutil.TryReceive(ctx, t, doneChan)
		require.Equal(t, "yes", resp)
	})

	t.Run("Skip", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ptty := ptytest.New(t)
		var buf bytes.Buffer

		// Copy all data written out to a buffer. When we close the ptty, we can
		// no longer read from the ptty.Output(), but we can read what was
		// written to the buffer.
		dataRead, doneReading := context.WithCancel(ctx)
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
			resp, err := newPrompt(ctx, ptty, cliui.PromptOptions{
				Text:      "ShouldNotSeeThis",
				IsConfirm: true,
			}, func(inv *serpent.Invocation) {
				inv.Command.Options = append(inv.Command.Options, cliui.SkipPromptOption())
				inv.Args = []string{"-y"}
			})
			assert.NoError(t, err)
			doneChan <- resp
		}()

		resp := testutil.TryReceive(ctx, t, doneChan)
		require.Equal(t, "yes", resp)
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
		ctx := testutil.Context(t, testutil.WaitShort)
		ptty := ptytest.New(t)
		doneChan := make(chan string)
		go func() {
			resp, err := newPrompt(ctx, ptty, cliui.PromptOptions{
				Text: "Example",
			}, nil)
			assert.NoError(t, err)
			doneChan <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine("{}")
		resp := testutil.TryReceive(ctx, t, doneChan)
		require.Equal(t, "{}", resp)
	})

	t.Run("BadJSON", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ptty := ptytest.New(t)
		doneChan := make(chan string)
		go func() {
			resp, err := newPrompt(ctx, ptty, cliui.PromptOptions{
				Text: "Example",
			}, nil)
			assert.NoError(t, err)
			doneChan <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine("{a")
		resp := testutil.TryReceive(ctx, t, doneChan)
		require.Equal(t, "{a", resp)
	})

	t.Run("MultilineJSON", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ptty := ptytest.New(t)
		doneChan := make(chan string)
		go func() {
			resp, err := newPrompt(ctx, ptty, cliui.PromptOptions{
				Text: "Example",
			}, nil)
			assert.NoError(t, err)
			doneChan <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine(`{
"test": "wow"
}`)
		resp := testutil.TryReceive(ctx, t, doneChan)
		require.Equal(t, `{"test":"wow"}`, resp)
	})

	t.Run("InvalidValid", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ptty := ptytest.New(t)
		doneChan := make(chan string)
		go func() {
			resp, err := newPrompt(ctx, ptty, cliui.PromptOptions{
				Text: "Example",
				Validate: func(s string) error {
					t.Logf("validate: %q", s)
					if s != "valid" {
						return xerrors.New("invalid")
					}
					return nil
				},
			}, nil)
			assert.NoError(t, err)
			doneChan <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine("foo\nbar\nbaz\n\n\nvalid\n")
		resp := testutil.TryReceive(ctx, t, doneChan)
		require.Equal(t, "valid", resp)
	})

	t.Run("MaskedSecret", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ptty := ptytest.New(t)
		doneChan := make(chan string)
		go func() {
			resp, err := newPrompt(ctx, ptty, cliui.PromptOptions{
				Text:   "Password:",
				Secret: true,
			}, nil)
			assert.NoError(t, err)
			doneChan <- resp
		}()
		ptty.ExpectMatch("Password: ")

		ptty.WriteLine("test")

		resp := testutil.TryReceive(ctx, t, doneChan)
		require.Equal(t, "test", resp)
	})

	t.Run("UTF8Password", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		ptty := ptytest.New(t)
		doneChan := make(chan string)
		go func() {
			resp, err := newPrompt(ctx, ptty, cliui.PromptOptions{
				Text:   "Password:",
				Secret: true,
			}, nil)
			assert.NoError(t, err)
			doneChan <- resp
		}()
		ptty.ExpectMatch("Password: ")

		ptty.WriteLine("和製漢字")

		resp := testutil.TryReceive(ctx, t, doneChan)
		require.Equal(t, "和製漢字", resp)
	})
}

func newPrompt(ctx context.Context, ptty *ptytest.PTY, opts cliui.PromptOptions, invOpt func(inv *serpent.Invocation)) (string, error) {
	value := ""
	cmd := &serpent.Command{
		Handler: func(inv *serpent.Invocation) error {
			var err error
			value, err = cliui.Prompt(inv, opts)
			return err
		},
	}

	inv := cmd.Invoke()
	// Optionally modify the cmd
	if invOpt != nil {
		invOpt(inv)
	}
	inv.Stdout = ptty.Output()
	inv.Stderr = ptty.Output()
	inv.Stdin = ptty.Input()
	return value, inv.WithContext(ctx).Run()
}

func TestPasswordTerminalState(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS") == "1" {
		passwordHelper()
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on windows. PTY doesn't read ptty.Write correctly.")
	}
	t.Parallel()

	ptty := ptytest.New(t)

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
	ptty.Write('t')
	ptty.Write('e')
	ptty.Write('s')
	ptty.Write('t')
	ptty.ExpectMatch("****")

	err = process.Signal(os.Interrupt)
	require.NoError(t, err)
	_, err = process.Wait()
	require.NoError(t, err)
}

// nolint:unused
func passwordHelper() {
	cmd := &serpent.Command{
		Handler: func(inv *serpent.Invocation) error {
			cliui.Prompt(inv, cliui.PromptOptions{
				Text:   "Password:",
				Secret: true,
			})
			return nil
		},
	}
	err := cmd.Invoke().WithOS().Run()
	if err != nil {
		panic(err)
	}
}
