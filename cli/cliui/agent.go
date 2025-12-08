package cliui

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"tailscale.com/tailcfg"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/healthsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
)

var errAgentShuttingDown = xerrors.New("agent is shutting down")

// fetchAgentResult is used to pass agent fetch results through channels.
type fetchAgentResult struct {
	agent codersdk.WorkspaceAgent
	err   error
}

type AgentOptions struct {
	FetchInterval time.Duration
	Fetch         func(ctx context.Context, agentID uuid.UUID) (codersdk.WorkspaceAgent, error)
	FetchLogs     func(ctx context.Context, agentID uuid.UUID, after int64, follow bool) (<-chan []codersdk.WorkspaceAgentLog, io.Closer, error)
	Wait          bool // If true, wait for the agent to be ready (startup script).
	DocsURL       string
}

// agentWaiter encapsulates the state machine for waiting on a workspace agent.
type agentWaiter struct {
	opts       AgentOptions
	sw         *stageWriter
	logSources map[uuid.UUID]codersdk.WorkspaceAgentLogSource
	fetchAgent func(context.Context) (codersdk.WorkspaceAgent, error)
}

// Agent displays a spinning indicator that waits for a workspace agent to connect.
func Agent(ctx context.Context, writer io.Writer, agentID uuid.UUID, opts AgentOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if opts.FetchInterval == 0 {
		opts.FetchInterval = 500 * time.Millisecond
	}
	if opts.FetchLogs == nil {
		opts.FetchLogs = func(_ context.Context, _ uuid.UUID, _ int64, _ bool) (<-chan []codersdk.WorkspaceAgentLog, io.Closer, error) {
			c := make(chan []codersdk.WorkspaceAgentLog)
			close(c)
			return c, closeFunc(func() error { return nil }), nil
		}
	}

	fetchedAgent := make(chan fetchAgentResult, 1)
	go func() {
		t := time.NewTimer(0)
		defer t.Stop()

		startTime := time.Now()
		baseInterval := opts.FetchInterval

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				agent, err := opts.Fetch(ctx, agentID)
				select {
				case <-fetchedAgent:
				default:
				}
				if err != nil {
					fetchedAgent <- fetchAgentResult{err: xerrors.Errorf("fetch workspace agent: %w", err)}
					return
				}
				fetchedAgent <- fetchAgentResult{agent: agent}

				// Adjust the interval based on how long we've been waiting.
				elapsed := time.Since(startTime)
				currentInterval := GetProgressiveInterval(baseInterval, elapsed)
				t.Reset(currentInterval)
			}
		}
	}()
	fetch := func(ctx context.Context) (codersdk.WorkspaceAgent, error) {
		select {
		case <-ctx.Done():
			return codersdk.WorkspaceAgent{}, ctx.Err()
		case f := <-fetchedAgent:
			if f.err != nil {
				return codersdk.WorkspaceAgent{}, f.err
			}
			return f.agent, nil
		}
	}

	agent, err := fetch(ctx)
	if err != nil {
		return xerrors.Errorf("fetch: %w", err)
	}
	logSources := map[uuid.UUID]codersdk.WorkspaceAgentLogSource{}
	for _, source := range agent.LogSources {
		logSources[source.ID] = source
	}

	w := &agentWaiter{
		opts:       opts,
		sw:         &stageWriter{w: writer},
		logSources: logSources,
		fetchAgent: fetch,
	}

	return w.wait(ctx, agent, fetchedAgent)
}

// wait runs the main state machine loop.
func (aw *agentWaiter) wait(ctx context.Context, agent codersdk.WorkspaceAgent, fetchedAgent chan fetchAgentResult) error {
	var err error
	// Track whether we've gone through a wait state, which determines if we
	// should show startup logs when connected.
	waitedForConnection := false

	for {
		// It doesn't matter if we're connected or not, if the agent is
		// shutting down, we don't know if it's coming back.
		if agent.LifecycleState.ShuttingDown() {
			return errAgentShuttingDown
		}

		switch agent.Status {
		case codersdk.WorkspaceAgentConnecting, codersdk.WorkspaceAgentTimeout:
			agent, err = aw.waitForConnection(ctx, agent)
			if err != nil {
				return err
			}
			// Since we were waiting for the agent to connect, also show
			// startup logs if applicable.
			waitedForConnection = true

		case codersdk.WorkspaceAgentConnected:
			return aw.handleConnected(ctx, agent, waitedForConnection, fetchedAgent)

		case codersdk.WorkspaceAgentDisconnected:
			agent, waitedForConnection, err = aw.waitForReconnection(ctx, agent)
			if err != nil {
				return err
			}
		}
	}
}

// waitForConnection handles the Connecting/Timeout states.
// Returns when agent transitions to Connected or Disconnected.
func (aw *agentWaiter) waitForConnection(ctx context.Context, agent codersdk.WorkspaceAgent) (codersdk.WorkspaceAgent, error) {
	stage := "Waiting for the workspace agent to connect"
	aw.sw.Start(stage)

	agent, err := aw.pollWhile(ctx, agent, func(agent codersdk.WorkspaceAgent) bool {
		return agent.Status == codersdk.WorkspaceAgentConnecting
	})
	if err != nil {
		return agent, err
	}

	if agent.Status == codersdk.WorkspaceAgentTimeout {
		now := time.Now()
		aw.sw.Log(now, codersdk.LogLevelInfo, "The workspace agent is having trouble connecting, wait for it to connect or restart your workspace.")
		aw.sw.Log(now, codersdk.LogLevelInfo, troubleshootingMessage(agent, fmt.Sprintf("%s/admin/templates/troubleshooting#agent-connection-issues", aw.opts.DocsURL)))
		agent, err = aw.pollWhile(ctx, agent, func(agent codersdk.WorkspaceAgent) bool {
			return agent.Status == codersdk.WorkspaceAgentTimeout
		})
		if err != nil {
			return agent, err
		}
	}

	aw.sw.Complete(stage, agent.FirstConnectedAt.Sub(agent.CreatedAt))
	return agent, nil
}

// handleConnected handles the Connected state and startup script logic.
// This is a terminal state, returns nil on success or error on failure.
//
//nolint:revive // Control flag is acceptable for internal method.
func (aw *agentWaiter) handleConnected(ctx context.Context, agent codersdk.WorkspaceAgent, showStartupLogs bool, fetchedAgent chan fetchAgentResult) error {
	if !showStartupLogs && agent.LifecycleState == codersdk.WorkspaceAgentLifecycleReady {
		// The workspace is ready, there's nothing to do but connect.
		return nil
	}

	// Determine if we should follow/stream logs (blocking mode).
	follow := aw.opts.Wait && agent.LifecycleState.Starting()

	stage := "Running workspace agent startup scripts"
	if !follow {
		stage += " (non-blocking)"
	}
	aw.sw.Start(stage)

	if follow {
		aw.sw.Log(time.Time{}, codersdk.LogLevelInfo, "==> ℹ︎ To connect immediately, reconnect with --wait=no or CODER_SSH_WAIT=no, see --help for more information.")
	}

	agent, err := aw.streamLogs(ctx, agent, follow, fetchedAgent)
	if err != nil {
		return err
	}

	// If we were following, wait until startup completes.
	if follow {
		agent, err = aw.pollWhile(ctx, agent, func(agent codersdk.WorkspaceAgent) bool {
			return agent.LifecycleState.Starting()
		})
		if err != nil {
			return err
		}
	}

	// Handle final lifecycle state.
	switch agent.LifecycleState {
	case codersdk.WorkspaceAgentLifecycleReady:
		aw.sw.Complete(stage, safeDuration(aw.sw, agent.ReadyAt, agent.StartedAt))
	case codersdk.WorkspaceAgentLifecycleStartTimeout:
		// Backwards compatibility: Avoid printing warning if
		// coderd is old and doesn't set ReadyAt for timeouts.
		if agent.ReadyAt == nil {
			aw.sw.Fail(stage, 0)
		} else {
			aw.sw.Fail(stage, safeDuration(aw.sw, agent.ReadyAt, agent.StartedAt))
		}
		aw.sw.Log(time.Time{}, codersdk.LogLevelWarn, "Warning: A startup script timed out and your workspace may be incomplete.")
	case codersdk.WorkspaceAgentLifecycleStartError:
		aw.sw.Fail(stage, safeDuration(aw.sw, agent.ReadyAt, agent.StartedAt))
		aw.sw.Log(time.Time{}, codersdk.LogLevelWarn, "Warning: A startup script exited with an error and your workspace may be incomplete.")
		aw.sw.Log(time.Time{}, codersdk.LogLevelWarn, troubleshootingMessage(agent, fmt.Sprintf("%s/admin/templates/troubleshooting#startup-script-exited-with-an-error", aw.opts.DocsURL)))
	default:
		switch {
		case agent.LifecycleState.Starting():
			aw.sw.Log(time.Time{}, codersdk.LogLevelWarn, "Notice: The startup scripts are still running and your workspace may be incomplete.")
			aw.sw.Log(time.Time{}, codersdk.LogLevelWarn, troubleshootingMessage(agent, fmt.Sprintf("%s/admin/templates/troubleshooting#your-workspace-may-be-incomplete", aw.opts.DocsURL)))
			// Note: We don't complete or fail the stage here, it's
			// intentionally left open to indicate this stage didn't
			// complete.
		case agent.LifecycleState.ShuttingDown():
			// We no longer know if the startup script failed or not,
			// but we need to tell the user something.
			aw.sw.Complete(stage, safeDuration(aw.sw, agent.ReadyAt, agent.StartedAt))
			return errAgentShuttingDown
		}
	}

	return nil
}

// streamLogs handles streaming or fetching startup logs.
//
//nolint:revive // Control flag is acceptable for internal method.
func (aw *agentWaiter) streamLogs(ctx context.Context, agent codersdk.WorkspaceAgent, follow bool, fetchedAgent chan fetchAgentResult) (codersdk.WorkspaceAgent, error) {
	logStream, logsCloser, err := aw.opts.FetchLogs(ctx, agent.ID, 0, follow)
	if err != nil {
		return agent, xerrors.Errorf("fetch workspace agent startup logs: %w", err)
	}
	defer logsCloser.Close()

	var lastLog codersdk.WorkspaceAgentLog

	// If not following, we don't need to watch for agent state changes.
	var fetchedAgentWhileFollowing chan fetchAgentResult
	if follow {
		fetchedAgentWhileFollowing = fetchedAgent
	}

	for {
		select {
		case <-ctx.Done():
			return agent, ctx.Err()
		case f := <-fetchedAgentWhileFollowing:
			if f.err != nil {
				return agent, xerrors.Errorf("fetch: %w", f.err)
			}
			agent = f.agent

			// If the agent is no longer starting, stop following
			// logs because FetchLogs will keep streaming forever.
			// We do one last non-follow request to ensure we have
			// fetched all logs.
			if !agent.LifecycleState.Starting() {
				_ = logsCloser.Close()
				fetchedAgentWhileFollowing = nil

				logStream, logsCloser, err = aw.opts.FetchLogs(ctx, agent.ID, lastLog.ID, false)
				if err != nil {
					return agent, xerrors.Errorf("fetch workspace agent startup logs: %w", err)
				}
				// Logs are already primed, so we can call close.
				_ = logsCloser.Close()
			}
		case logs, ok := <-logStream:
			if !ok {
				return agent, nil
			}
			for _, log := range logs {
				source, hasSource := aw.logSources[log.SourceID]
				output := log.Output
				if hasSource && source.DisplayName != "" {
					output = source.DisplayName + ": " + output
				}
				aw.sw.Log(log.CreatedAt, log.Level, output)
				lastLog = log
			}
		}
	}
}

// waitForReconnection handles the Disconnected state.
// Returns when agent reconnects along with whether to show startup logs.
func (aw *agentWaiter) waitForReconnection(ctx context.Context, agent codersdk.WorkspaceAgent) (codersdk.WorkspaceAgent, bool, error) {
	// If the agent was still starting during disconnect, we'll
	// show startup logs.
	showStartupLogs := agent.LifecycleState.Starting()

	stage := "The workspace agent lost connection"
	aw.sw.Start(stage)
	aw.sw.Log(time.Now(), codersdk.LogLevelWarn, "Wait for it to reconnect or restart your workspace.")
	aw.sw.Log(time.Now(), codersdk.LogLevelWarn, troubleshootingMessage(agent, fmt.Sprintf("%s/admin/templates/troubleshooting#agent-connection-issues", aw.opts.DocsURL)))

	disconnectedAt := agent.DisconnectedAt
	agent, err := aw.pollWhile(ctx, agent, func(agent codersdk.WorkspaceAgent) bool {
		return agent.Status == codersdk.WorkspaceAgentDisconnected
	})
	if err != nil {
		return agent, showStartupLogs, err
	}
	aw.sw.Complete(stage, safeDuration(aw.sw, agent.LastConnectedAt, disconnectedAt))

	return agent, showStartupLogs, nil
}

// pollWhile polls the agent while the condition is true. It fetches the agent
// on each iteration and returns the updated agent when the condition is false,
// the context is canceled, or an error occurs.
func (aw *agentWaiter) pollWhile(ctx context.Context, agent codersdk.WorkspaceAgent, cond func(agent codersdk.WorkspaceAgent) bool) (codersdk.WorkspaceAgent, error) {
	var err error
	for cond(agent) {
		agent, err = aw.fetchAgent(ctx)
		if err != nil {
			return agent, xerrors.Errorf("fetch: %w", err)
		}
	}
	if err = ctx.Err(); err != nil {
		return agent, err
	}
	return agent, nil
}

func troubleshootingMessage(agent codersdk.WorkspaceAgent, url string) string {
	m := "For more information and troubleshooting, see " + url
	if agent.TroubleshootingURL != "" {
		m += " and " + agent.TroubleshootingURL
	}
	return m
}

// safeDuration returns a-b. If a or b is nil, it returns 0.
// This is because we often dereference a time pointer, which can
// cause a panic. These dereferences are used to calculate durations,
// which are not critical, and therefor should not break things
// when it fails.
// A panic has been observed in a test.
func safeDuration(sw *stageWriter, a, b *time.Time) time.Duration {
	if a == nil || b == nil {
		if sw != nil {
			// Ideally the message includes which fields are <nil>, but you can
			// use the surrounding log lines to figure that out. And passing more
			// params makes this unwieldy.
			sw.Log(time.Now(), codersdk.LogLevelWarn, "Warning: Failed to calculate duration from a time being <nil>.")
		}
		return 0
	}
	return a.Sub(*b)
}

// GetProgressiveInterval returns an interval that increases over time.
// The interval starts at baseInterval and increases to
// a maximum of baseInterval * 16 over time.
func GetProgressiveInterval(baseInterval time.Duration, elapsed time.Duration) time.Duration {
	switch {
	case elapsed < 60*time.Second:
		return baseInterval // 500ms for first 60 seconds
	case elapsed < 2*time.Minute:
		return baseInterval * 2 // 1s for next 1 minute
	case elapsed < 5*time.Minute:
		return baseInterval * 4 // 2s for next 3 minutes
	case elapsed < 10*time.Minute:
		return baseInterval * 8 // 4s for next 5 minutes
	default:
		return baseInterval * 16 // 8s after 10 minutes
	}
}

type closeFunc func() error

func (c closeFunc) Close() error {
	return c()
}

func PeerDiagnostics(w io.Writer, d tailnet.PeerDiagnostics) {
	if d.PreferredDERP > 0 {
		rn, ok := d.DERPRegionNames[d.PreferredDERP]
		if !ok {
			rn = "unknown"
		}
		_, _ = fmt.Fprintf(w, "✔ preferred DERP region: %d (%s)\n", d.PreferredDERP, rn)
	} else {
		_, _ = fmt.Fprint(w, "✘ not connected to DERP\n")
	}
	if d.SentNode {
		_, _ = fmt.Fprint(w, "✔ sent local data to Coder networking coordinator\n")
	} else {
		_, _ = fmt.Fprint(w, "✘ have not sent local data to Coder networking coordinator\n")
	}
	if d.ReceivedNode != nil {
		dp := d.ReceivedNode.DERP
		dn := ""
		// should be 127.3.3.40:N where N is the DERP region
		ap := strings.Split(dp, ":")
		if len(ap) == 2 {
			dp = ap[1]
			di, err := strconv.Atoi(dp)
			if err == nil {
				var ok bool
				dn, ok = d.DERPRegionNames[di]
				if ok {
					dn = fmt.Sprintf("(%s)", dn)
				} else {
					dn = "(unknown)"
				}
			}
		}
		_, _ = fmt.Fprintf(w,
			"✔ received remote agent data from Coder networking coordinator\n    preferred DERP region: %s %s\n    endpoints: %s\n",
			dp, dn, strings.Join(d.ReceivedNode.Endpoints, ", "))
	} else {
		_, _ = fmt.Fprint(w, "✘ have not received remote agent data from Coder networking coordinator\n")
	}
	if !d.LastWireguardHandshake.IsZero() {
		ago := time.Since(d.LastWireguardHandshake)
		symbol := "✔"
		// wireguard is supposed to refresh handshake on 5 minute intervals
		if ago > 5*time.Minute {
			symbol = "⚠"
		}
		_, _ = fmt.Fprintf(w, "%s Wireguard handshake %s ago\n", symbol, ago.Round(time.Second))
	} else {
		_, _ = fmt.Fprint(w, "✘ Wireguard is not connected\n")
	}
}

type ConnDiags struct {
	ConnInfo           workspacesdk.AgentConnectionInfo
	PingP2P            bool
	DisableDirect      bool
	LocalNetInfo       *tailcfg.NetInfo
	LocalInterfaces    *healthsdk.InterfacesReport
	AgentNetcheck      *healthsdk.AgentNetcheckReport
	ClientIPIsAWS      bool
	AgentIPIsAWS       bool
	Verbose            bool
	TroubleshootingURL string
}

func (d ConnDiags) Write(w io.Writer) {
	_, _ = fmt.Fprintln(w, "")
	general, client, agent := d.splitDiagnostics()
	for _, msg := range general {
		_, _ = fmt.Fprintln(w, msg)
	}
	if len(general) > 0 {
		_, _ = fmt.Fprintln(w, "")
	}
	if len(client) > 0 {
		_, _ = fmt.Fprint(w, "Possible client-side issues with direct connection:\n\n")
		for _, msg := range client {
			_, _ = fmt.Fprintf(w, " - %s\n\n", msg)
		}
	}
	if len(agent) > 0 {
		_, _ = fmt.Fprint(w, "Possible agent-side issues with direct connections:\n\n")
		for _, msg := range agent {
			_, _ = fmt.Fprintf(w, " - %s\n\n", msg)
		}
	}
}

func (d ConnDiags) splitDiagnostics() (general, client, agent []string) {
	if d.AgentNetcheck != nil {
		for _, msg := range d.AgentNetcheck.Interfaces.Warnings {
			agent = append(agent, msg.Message)
		}
		if len(d.AgentNetcheck.Interfaces.Warnings) > 0 {
			agent[len(agent)-1] += fmt.Sprintf("\n%s#low-mtu", d.TroubleshootingURL)
		}
	}

	if d.LocalInterfaces != nil {
		for _, msg := range d.LocalInterfaces.Warnings {
			client = append(client, msg.Message)
		}
		if len(d.LocalInterfaces.Warnings) > 0 {
			client[len(client)-1] += fmt.Sprintf("\n%s#low-mtu", d.TroubleshootingURL)
		}
	}

	if d.PingP2P && !d.Verbose {
		return general, client, agent
	}

	if d.DisableDirect {
		general = append(general, "❗ Direct connections are disabled locally, by `--disable-direct-connections` or `CODER_DISABLE_DIRECT_CONNECTIONS`.\n"+
			"   They may still be established over a private network.")
		if !d.Verbose {
			return general, client, agent
		}
	}

	if d.ConnInfo.DisableDirectConnections {
		general = append(general,
			fmt.Sprintf("❗ Your Coder administrator has blocked direct connections\n   %s#disabled-deployment-wide", d.TroubleshootingURL))
		if !d.Verbose {
			return general, client, agent
		}
	}

	if !d.ConnInfo.DERPMap.HasSTUN() {
		general = append(general,
			fmt.Sprintf("❗ The DERP map is not configured to use STUN\n   %s#no-stun-servers", d.TroubleshootingURL))
	} else if d.LocalNetInfo != nil && !d.LocalNetInfo.UDP {
		client = append(client,
			fmt.Sprintf("Client could not connect to STUN over UDP\n   %s#udp-blocked", d.TroubleshootingURL))
	}

	if d.LocalNetInfo != nil && d.LocalNetInfo.MappingVariesByDestIP.EqualBool(true) {
		client = append(client,
			fmt.Sprintf("Client is potentially behind a hard NAT, as multiple endpoints were retrieved from different STUN servers\n  %s#endpoint-dependent-nat-hard-nat", d.TroubleshootingURL))
	}

	if d.AgentNetcheck != nil && d.AgentNetcheck.NetInfo != nil {
		if d.AgentNetcheck.NetInfo.MappingVariesByDestIP.EqualBool(true) {
			agent = append(agent,
				fmt.Sprintf("Agent is potentially behind a hard NAT, as multiple endpoints were retrieved from different STUN servers\n   %s#endpoint-dependent-nat-hard-nat", d.TroubleshootingURL))
		}
		if !d.AgentNetcheck.NetInfo.UDP {
			agent = append(agent,
				fmt.Sprintf("Agent could not connect to STUN over UDP\n   %s#udp-blocked", d.TroubleshootingURL))
		}
	}

	if d.ClientIPIsAWS {
		client = append(client,
			fmt.Sprintf("Client IP address is within an AWS range (AWS uses hard NAT)\n   %s#endpoint-dependent-nat-hard-nat", d.TroubleshootingURL))
	}

	if d.AgentIPIsAWS {
		agent = append(agent,
			fmt.Sprintf("Agent IP address is within an AWS range (AWS uses hard NAT)\n   %s#endpoint-dependent-nat-hard-nat", d.TroubleshootingURL))
	}

	return general, client, agent
}
