package cliui_test

import (
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/pty/ptytest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestPrompt(t *testing.T) {
	t.Parallel()
	t.Run("Success", func(t *testing.T) {
		ptty := ptytest.New(t)
		ch := make(chan string, 0)
		go func() {
			resp, err := prompt(ptty, cliui.PromptOptions{
				Prompt: "Example",
			})
			require.NoError(t, err)
			fmt.Printf("We got it!\n")
			ch <- resp
		}()
		ptty.ExpectMatch("Example")
		ptty.WriteLine("hello")
		resp := <-ch
		require.Equal(t, "hello", resp)
	})
}

func prompt(ptty *ptytest.PTY, opts cliui.PromptOptions) (string, error) {
	cmd := &cobra.Command{}
	cmd.SetOutput(ptty.Output())
	cmd.SetIn(ptty.Input())
	return cliui.Prompt(cmd, opts)
}
