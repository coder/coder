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
	"github.com/coder/coder/testutil"
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

func TestAgentTimeoutWithTroubleshootingURL(t *testing.T) {
	t.Parallel()

	ctx, _ := testutil.Context(t)

	wantURL := "https://coder.com/troubleshoot"

	var connected, timeout atomic.Bool
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, args []string) error {
			err := cliui.Agent(cmd.Context(), cmd.OutOrStdout(), cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(ctx context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status:             codersdk.WorkspaceAgentConnecting,
						TroubleshootingURL: "https://coder.com/troubleshoot",
					}
					switch {
					case connected.Load():
						agent.Status = codersdk.WorkspaceAgentConnected
					case timeout.Load():
						agent.Status = codersdk.WorkspaceAgentTimeout
					}
					return agent, nil
				},
				FetchInterval: time.Millisecond,
				WarnInterval:  5 * time.Millisecond,
			})
			return err
		},
	}
	ptty := ptytest.New(t)
	cmd.SetOutput(ptty.Output())
	cmd.SetIn(ptty.Input())
	done := make(chan struct{})
	go func() {
		defer close(done)
		err := cmd.ExecuteContext(ctx)
		assert.NoError(t, err)
	}()
	ptty.ExpectMatch("Don't panic")
	timeout.Store(true)
	ptty.ExpectMatch(wantURL)
	connected.Store(true)
	<-done
}
