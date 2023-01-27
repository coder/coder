package cliui_test

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty/ptytest"
	"github.com/coder/coder/testutil"
)

func TestAgent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	var disconnected atomic.Bool
	ptty := ptytest.New(t)
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := cliui.Agent(cmd.Context(), cmd.OutOrStdout(), cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(_ context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status:           codersdk.WorkspaceAgentDisconnected,
						LoginBeforeReady: true,
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
	ptty.ExpectMatchContext(ctx, "lost connection")
	disconnected.Store(true)
	<-done
}

func TestAgent_TimeoutWithTroubleshootingURL(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	wantURL := "https://coder.com/troubleshoot"

	var connected, timeout atomic.Bool
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := cliui.Agent(cmd.Context(), cmd.OutOrStdout(), cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(_ context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status:             codersdk.WorkspaceAgentConnecting,
						TroubleshootingURL: wantURL,
						LoginBeforeReady:   true,
					}
					switch {
					case !connected.Load() && timeout.Load():
						agent.Status = codersdk.WorkspaceAgentTimeout
					case connected.Load():
						agent.Status = codersdk.WorkspaceAgentConnected
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
	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(ctx)
	}()
	ptty.ExpectMatchContext(ctx, "Don't panic, your workspace is booting")
	timeout.Store(true)
	ptty.ExpectMatchContext(ctx, wantURL)
	connected.Store(true)
	require.NoError(t, <-done)
}

func TestAgent_StartupTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	wantURL := "https://coder.com/this-is-a-really-long-troubleshooting-url-that-should-not-wrap"

	var status, state atomic.String
	setStatus := func(s codersdk.WorkspaceAgentStatus) { status.Store(string(s)) }
	setState := func(s codersdk.WorkspaceAgentLifecycle) { state.Store(string(s)) }
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := cliui.Agent(cmd.Context(), cmd.OutOrStdout(), cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(_ context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status:             codersdk.WorkspaceAgentConnecting,
						LoginBeforeReady:   false,
						LifecycleState:     codersdk.WorkspaceAgentLifecycleCreated,
						TroubleshootingURL: wantURL,
					}

					if s := status.Load(); s != "" {
						agent.Status = codersdk.WorkspaceAgentStatus(s)
					}
					if s := state.Load(); s != "" {
						agent.LifecycleState = codersdk.WorkspaceAgentLifecycle(s)
					}
					return agent, nil
				},
				FetchInterval: time.Millisecond,
				WarnInterval:  time.Millisecond,
				NoWait:        false,
			})
			return err
		},
	}

	ptty := ptytest.New(t)
	cmd.SetOutput(ptty.Output())
	cmd.SetIn(ptty.Input())
	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(ctx)
	}()
	setStatus(codersdk.WorkspaceAgentConnecting)
	ptty.ExpectMatchContext(ctx, "Don't panic, your workspace is booting")
	setStatus(codersdk.WorkspaceAgentConnected)
	setState(codersdk.WorkspaceAgentLifecycleStarting)
	ptty.ExpectMatchContext(ctx, "workspace is getting ready")
	setState(codersdk.WorkspaceAgentLifecycleStartTimeout)
	ptty.ExpectMatchContext(ctx, "is taking longer")
	ptty.ExpectMatchContext(ctx, wantURL)
	setState(codersdk.WorkspaceAgentLifecycleReady)
	require.NoError(t, <-done)
}

func TestAgent_StartErrorExit(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	wantURL := "https://coder.com/this-is-a-really-long-troubleshooting-url-that-should-not-wrap"

	var status, state atomic.String
	setStatus := func(s codersdk.WorkspaceAgentStatus) { status.Store(string(s)) }
	setState := func(s codersdk.WorkspaceAgentLifecycle) { state.Store(string(s)) }
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := cliui.Agent(cmd.Context(), cmd.OutOrStdout(), cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(_ context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status:             codersdk.WorkspaceAgentConnecting,
						LoginBeforeReady:   false,
						LifecycleState:     codersdk.WorkspaceAgentLifecycleCreated,
						TroubleshootingURL: wantURL,
					}

					if s := status.Load(); s != "" {
						agent.Status = codersdk.WorkspaceAgentStatus(s)
					}
					if s := state.Load(); s != "" {
						agent.LifecycleState = codersdk.WorkspaceAgentLifecycle(s)
					}
					return agent, nil
				},
				FetchInterval: time.Millisecond,
				WarnInterval:  60 * time.Second,
				NoWait:        false,
			})
			return err
		},
	}

	ptty := ptytest.New(t)
	cmd.SetOutput(ptty.Output())
	cmd.SetIn(ptty.Input())
	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(ctx)
	}()
	setStatus(codersdk.WorkspaceAgentConnected)
	setState(codersdk.WorkspaceAgentLifecycleStarting)
	ptty.ExpectMatchContext(ctx, "to become ready...")
	setState(codersdk.WorkspaceAgentLifecycleStartError)
	ptty.ExpectMatchContext(ctx, "ran into a problem")
	err := <-done
	require.ErrorIs(t, err, cliui.AgentStartError, "lifecycle start_error should exit with error")
}

func TestAgent_NoWait(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	wantURL := "https://coder.com/this-is-a-really-long-troubleshooting-url-that-should-not-wrap"

	var status, state atomic.String
	setStatus := func(s codersdk.WorkspaceAgentStatus) { status.Store(string(s)) }
	setState := func(s codersdk.WorkspaceAgentLifecycle) { state.Store(string(s)) }
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := cliui.Agent(cmd.Context(), cmd.OutOrStdout(), cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(_ context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status:             codersdk.WorkspaceAgentConnecting,
						LoginBeforeReady:   false,
						LifecycleState:     codersdk.WorkspaceAgentLifecycleCreated,
						TroubleshootingURL: wantURL,
					}

					if s := status.Load(); s != "" {
						agent.Status = codersdk.WorkspaceAgentStatus(s)
					}
					if s := state.Load(); s != "" {
						agent.LifecycleState = codersdk.WorkspaceAgentLifecycle(s)
					}
					return agent, nil
				},
				FetchInterval: time.Millisecond,
				WarnInterval:  time.Second,
				NoWait:        true,
			})
			return err
		},
	}

	ptty := ptytest.New(t)
	cmd.SetOutput(ptty.Output())
	cmd.SetIn(ptty.Input())
	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(ctx)
	}()
	setStatus(codersdk.WorkspaceAgentConnecting)
	ptty.ExpectMatchContext(ctx, "Don't panic, your workspace is booting")

	setStatus(codersdk.WorkspaceAgentConnected)
	require.NoError(t, <-done, "created - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleStarting)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	require.NoError(t, <-done, "starting - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleStartTimeout)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	require.NoError(t, <-done, "start timeout - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleStartError)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	require.NoError(t, <-done, "start error - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleReady)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	require.NoError(t, <-done, "ready - should exit early")
}

func TestAgent_LoginBeforeReadyEnabled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	wantURL := "https://coder.com/this-is-a-really-long-troubleshooting-url-that-should-not-wrap"

	var status, state atomic.String
	setStatus := func(s codersdk.WorkspaceAgentStatus) { status.Store(string(s)) }
	setState := func(s codersdk.WorkspaceAgentLifecycle) { state.Store(string(s)) }
	cmd := &cobra.Command{
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := cliui.Agent(cmd.Context(), cmd.OutOrStdout(), cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(_ context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status:             codersdk.WorkspaceAgentConnecting,
						LoginBeforeReady:   true,
						LifecycleState:     codersdk.WorkspaceAgentLifecycleCreated,
						TroubleshootingURL: wantURL,
					}

					if s := status.Load(); s != "" {
						agent.Status = codersdk.WorkspaceAgentStatus(s)
					}
					if s := state.Load(); s != "" {
						agent.LifecycleState = codersdk.WorkspaceAgentLifecycle(s)
					}
					return agent, nil
				},
				FetchInterval: time.Millisecond,
				WarnInterval:  time.Second,
				NoWait:        false,
			})
			return err
		},
	}

	ptty := ptytest.New(t)
	cmd.SetOutput(ptty.Output())
	cmd.SetIn(ptty.Input())
	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteContext(ctx)
	}()
	setStatus(codersdk.WorkspaceAgentConnecting)
	ptty.ExpectMatchContext(ctx, "Don't panic, your workspace is booting")

	setStatus(codersdk.WorkspaceAgentConnected)
	require.NoError(t, <-done, "created - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleStarting)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	require.NoError(t, <-done, "starting - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleStartTimeout)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	require.NoError(t, <-done, "start timeout - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleStartError)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	require.NoError(t, <-done, "start error - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleReady)
	go func() { done <- cmd.ExecuteContext(ctx) }()
	require.NoError(t, <-done, "ready - should exit early")
}
