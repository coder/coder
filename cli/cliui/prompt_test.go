package cliui_test

import (
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
			resp, err := prompt(ptty, cliui.PromptOptions{
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
			resp, err := prompt(ptty, cliui.PromptOptions{
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
}

func prompt(ptty *ptytest.PTY, opts cliui.PromptOptions) (string, error) {
	cmd := &cobra.Command{}
	cmd.SetOutput(ptty.Output())
	cmd.SetIn(ptty.Input().Reader)
	return cliui.Prompt(cmd, opts)
}
