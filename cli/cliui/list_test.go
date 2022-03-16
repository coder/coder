package cliui_test

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/pty/ptytest"
)

func TestList(t *testing.T) {
	t.Parallel()
	t.Run("Select", func(t *testing.T) {
		t.Parallel()
		ptty := ptytest.New(t)
		msgChan := make(chan string)
		go func() {
			resp, err := list(ptty, cliui.ListOptions{
				Title: "Example",
				Items: []cliui.ListItem{{
					ID:          "some",
					Title:       "Item",
					Description: "Description",
				}, {
					ID:          "another",
					Title:       "Another",
					Description: "Another one!",
				}},
			})
			require.NoError(t, err)
			msgChan <- resp
		}()
		ptty.ExpectMatch("Description")
		ptty.Write(ptytest.KeyDown)
		ptty.WriteEnter()
		require.Equal(t, "another", <-msgChan)
	})
}

func list(ptty *ptytest.PTY, opts cliui.ListOptions) (string, error) {
	value := ""
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			var err error
			value, err = cliui.List(cmd, opts)
			return err
		},
	}
	cmd.SetOutput(ptty.Output())
	cmd.SetIn(ptty.Input())
	return value, cmd.ExecuteContext(context.Background())
}
