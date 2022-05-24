package cliui_test

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty/ptytest"
)

func TestAgent(t *testing.T) {
	t.Parallel()
	var disconnected atomic.Bool
	ptty := ptytest.New(t)
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			err := cliui.Agent(cmd.Context(), cmd.OutOrStdout(), cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(ctx context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status: codersdk.WorkspaceAgentDisconnected,
					}
					if disconnected.Load() {
						agent.Status = codersdk.WorkspaceAgentConnected
					}
					return agent, nil
				},
				FetchInterval: time.Millisecond,
				WarnInterval:  10 * time.Millisecond,
			})
			return err
		},
	}
	cmd.SetOutput(ptty.Output())
	cmd.SetIn(ptty.Input())
	done := make(chan struct{})
	go func() {
		defer close(done)
		err := cmd.Execute()
		assert.NoError(t, err)
	}()
	ptty.ExpectMatch("lost connection")
	disconnected.Store(true)
	<-done
}
