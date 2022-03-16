package cliui_test

import (
	"context"
	"testing"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/pty/ptytest"
)

func TestSelect(t *testing.T) {
	t.Parallel()
	t.Run("Select", func(t *testing.T) {
		t.Parallel()
		ptty := ptytest.New(t)
		msgChan := make(chan string)
		go func() {
			resp, err := newSelect(ptty, cliui.SelectOptions{
				Options: []string{"First", "Second"},
			})
			require.NoError(t, err)
			msgChan <- resp
		}()
		ptty.ExpectMatch("Second")
		ptty.Write(promptui.KeyNext)
		ptty.WriteLine("")
		require.Equal(t, "Second", <-msgChan)
	})
}

func newSelect(ptty *ptytest.PTY, opts cliui.SelectOptions) (string, error) {
	value := ""
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			value, err = cliui.Select(cmd, opts)
			return err
		},
	}
	cmd.SetOutput(ptty.Output())
	cmd.SetIn(ptty.Input())
	return value, cmd.ExecuteContext(context.Background())
}
