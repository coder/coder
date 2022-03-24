package cliui_test

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/cliui"
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
		require.Equal(t, `{
"test": "wow"
}`, <-doneChan)
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
