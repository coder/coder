package cliui_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/coder/coder/cli/clibase"
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
	cmd := &clibase.Cmd{
		Handler: func(inv *clibase.Invocation) error {
			err := cliui.Agent(inv.Context(), inv.Stdout, cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(_ context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status:                codersdk.WorkspaceAgentDisconnected,
						StartupScriptBehavior: codersdk.WorkspaceAgentStartupScriptBehaviorNonBlocking,
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

	inv := cmd.Invoke()
	ptty.Attach(inv)
	done := make(chan struct{})
	go func() {
		defer close(done)
		err := inv.Run()
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
	cmd := &clibase.Cmd{
		Handler: func(inv *clibase.Invocation) error {
			err := cliui.Agent(inv.Context(), inv.Stdout, cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(_ context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status:                codersdk.WorkspaceAgentConnecting,
						TroubleshootingURL:    wantURL,
						StartupScriptBehavior: codersdk.WorkspaceAgentStartupScriptBehaviorNonBlocking,
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

	inv := cmd.Invoke()
	ptty.Attach(inv)
	done := make(chan error, 1)
	go func() {
		done <- inv.WithContext(ctx).Run()
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

	cmd := &clibase.Cmd{
		Handler: func(inv *clibase.Invocation) error {
			err := cliui.Agent(inv.Context(), inv.Stdout, cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(_ context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status:                codersdk.WorkspaceAgentConnecting,
						StartupScriptBehavior: codersdk.WorkspaceAgentStartupScriptBehaviorBlocking,
						LifecycleState:        codersdk.WorkspaceAgentLifecycleCreated,
						TroubleshootingURL:    wantURL,
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
				Wait:          true,
			})
			return err
		},
	}

	ptty := ptytest.New(t)

	inv := cmd.Invoke()
	ptty.Attach(inv)
	done := make(chan error, 1)
	go func() {
		done <- inv.WithContext(ctx).Run()
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
	cmd := &clibase.Cmd{
		Handler: func(inv *clibase.Invocation) error {
			err := cliui.Agent(inv.Context(), inv.Stdout, cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(_ context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status:                codersdk.WorkspaceAgentConnecting,
						StartupScriptBehavior: codersdk.WorkspaceAgentStartupScriptBehaviorBlocking,
						LifecycleState:        codersdk.WorkspaceAgentLifecycleCreated,
						TroubleshootingURL:    wantURL,
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
				Wait:          true,
			})
			return err
		},
	}

	ptty := ptytest.New(t)

	inv := cmd.Invoke()
	ptty.Attach(inv)
	done := make(chan error, 1)
	go func() {
		done <- inv.WithContext(ctx).Run()
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
	cmd := &clibase.Cmd{
		Handler: func(inv *clibase.Invocation) error {
			err := cliui.Agent(inv.Context(), inv.Stdout, cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(_ context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status:                codersdk.WorkspaceAgentConnecting,
						StartupScriptBehavior: codersdk.WorkspaceAgentStartupScriptBehaviorBlocking,
						LifecycleState:        codersdk.WorkspaceAgentLifecycleCreated,
						TroubleshootingURL:    wantURL,
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
				Wait:          false,
			})
			return err
		},
	}

	ptty := ptytest.New(t)

	inv := cmd.Invoke()
	ptty.Attach(inv)
	done := make(chan error, 1)
	go func() {
		done <- inv.WithContext(ctx).Run()
	}()
	setStatus(codersdk.WorkspaceAgentConnecting)
	ptty.ExpectMatchContext(ctx, "Don't panic, your workspace is booting")

	setStatus(codersdk.WorkspaceAgentConnected)
	require.NoError(t, <-done, "created - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleStarting)
	go func() { done <- inv.WithContext(ctx).Run() }()
	require.NoError(t, <-done, "starting - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleStartTimeout)
	go func() { done <- inv.WithContext(ctx).Run() }()
	require.NoError(t, <-done, "start timeout - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleStartError)
	go func() { done <- inv.WithContext(ctx).Run() }()
	require.NoError(t, <-done, "start error - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleReady)
	go func() { done <- inv.WithContext(ctx).Run() }()
	require.NoError(t, <-done, "ready - should exit early")
}

func TestAgent_StartupScriptBehaviorNonBlocking(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	wantURL := "https://coder.com/this-is-a-really-long-troubleshooting-url-that-should-not-wrap"

	var status, state atomic.String
	setStatus := func(s codersdk.WorkspaceAgentStatus) { status.Store(string(s)) }
	setState := func(s codersdk.WorkspaceAgentLifecycle) { state.Store(string(s)) }
	cmd := &clibase.Cmd{
		Handler: func(inv *clibase.Invocation) error {
			err := cliui.Agent(inv.Context(), inv.Stdout, cliui.AgentOptions{
				WorkspaceName: "example",
				Fetch: func(_ context.Context) (codersdk.WorkspaceAgent, error) {
					agent := codersdk.WorkspaceAgent{
						Status:                codersdk.WorkspaceAgentConnecting,
						StartupScriptBehavior: codersdk.WorkspaceAgentStartupScriptBehaviorNonBlocking,
						LifecycleState:        codersdk.WorkspaceAgentLifecycleCreated,
						TroubleshootingURL:    wantURL,
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
				Wait:          true,
			})
			return err
		},
	}

	inv := cmd.Invoke()

	ptty := ptytest.New(t)
	ptty.Attach(inv)
	done := make(chan error, 1)
	go func() {
		done <- inv.WithContext(ctx).Run()
	}()
	setStatus(codersdk.WorkspaceAgentConnecting)
	ptty.ExpectMatchContext(ctx, "Don't panic, your workspace is booting")

	setStatus(codersdk.WorkspaceAgentConnected)
	require.NoError(t, <-done, "created - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleStarting)
	go func() { done <- inv.WithContext(ctx).Run() }()
	require.NoError(t, <-done, "starting - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleStartTimeout)
	go func() { done <- inv.WithContext(ctx).Run() }()
	require.NoError(t, <-done, "start timeout - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleStartError)
	go func() { done <- inv.WithContext(ctx).Run() }()
	require.NoError(t, <-done, "start error - should exit early")

	setState(codersdk.WorkspaceAgentLifecycleReady)
	go func() { done <- inv.WithContext(ctx).Run() }()
	require.NoError(t, <-done, "ready - should exit early")
}
