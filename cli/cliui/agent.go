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

type AgentOptions struct {
	FetchInterval time.Duration
	Fetch         func(ctx context.Context, agentID uuid.UUID) (codersdk.WorkspaceAgent, error)
	FetchLogs     func(ctx context.Context, agentID uuid.UUID, after int64, follow bool) (<-chan []codersdk.WorkspaceAgentLog, io.Closer, error)
	Wait          bool // If true, wait for the agent to be ready (startup script).
	DocsURL       string
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

	type fetchAgent struct {
		agent codersdk.WorkspaceAgent
		err   error
	}
	fetchedAgent := make(chan fetchAgent, 1)
	go func() {
		t := time.NewTimer(0)
		defer t.Stop()

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
					fetchedAgent <- fetchAgent{err: xerrors.Errorf("fetch workspace agent: %w", err)}
					return
				}
				fetchedAgent <- fetchAgent{agent: agent}
				t.Reset(opts.FetchInterval)
			}
		}
	}()
	fetch := func() (codersdk.WorkspaceAgent, error) {
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

	agent, err := fetch()
	if err != nil {
		return xerrors.Errorf("fetch: %w", err)
	}
	logSources := map[uuid.UUID]codersdk.WorkspaceAgentLogSource{}
	for _, source := range agent.LogSources {
		logSources[source.ID] = source
	}

	sw := &stageWriter{w: writer}

	showStartupLogs := false
	for {
		// It doesn't matter if we're connected or not, if the agent is
		// shutting down, we don't know if it's coming back.
		if agent.LifecycleState.ShuttingDown() {
			return errAgentShuttingDown
		}

		switch agent.Status {
		case codersdk.WorkspaceAgentConnecting, codersdk.WorkspaceAgentTimeout:
			// Since we were waiting for the agent to connect, also show
			// startup logs if applicable.
			showStartupLogs = true

			stage := "Waiting for the workspace agent to connect"
			sw.Start(stage)
			for agent.Status == codersdk.WorkspaceAgentConnecting {
				if agent, err = fetch(); err != nil {
					return xerrors.Errorf("fetch: %w", err)
				}
			}

			if agent.Status == codersdk.WorkspaceAgentTimeout {
				now := time.Now()
				sw.Log(now, codersdk.LogLevelInfo, "The workspace agent is having trouble connecting, wait for it to connect or restart your workspace.")
				sw.Log(now, codersdk.LogLevelInfo, troubleshootingMessage(agent, fmt.Sprintf("%s/templates#agent-connection-issues", opts.DocsURL)))
				for agent.Status == codersdk.WorkspaceAgentTimeout {
					if agent, err = fetch(); err != nil {
						return xerrors.Errorf("fetch: %w", err)
					}
				}
			}
			sw.Complete(stage, agent.FirstConnectedAt.Sub(agent.CreatedAt))

		case codersdk.WorkspaceAgentConnected:
			if !showStartupLogs && agent.LifecycleState == codersdk.WorkspaceAgentLifecycleReady {
				// The workspace is ready, there's nothing to do but connect.
				return nil
			}

			stage := "Running workspace agent startup scripts"
			follow := opts.Wait && agent.LifecycleState.Starting()
			if !follow {
				stage += " (non-blocking)"
			}
			sw.Start(stage)
			if follow {
				sw.Log(time.Time{}, codersdk.LogLevelInfo, "==> ℹ︎ To connect immediately, reconnect with --wait=no or CODER_SSH_WAIT=no, see --help for more information.")
			}

			err = func() error { // Use func because of defer in for loop.
				logStream, logsCloser, err := opts.FetchLogs(ctx, agent.ID, 0, follow)
				if err != nil {
					return xerrors.Errorf("fetch workspace agent startup logs: %w", err)
				}
				defer logsCloser.Close()

				var lastLog codersdk.WorkspaceAgentLog
				fetchedAgentWhileFollowing := fetchedAgent
				if !follow {
					fetchedAgentWhileFollowing = nil
				}
				for {
					// This select is essentially and inline `fetch()`.
					select {
					case <-ctx.Done():
						return ctx.Err()
					case f := <-fetchedAgentWhileFollowing:
						if f.err != nil {
							return xerrors.Errorf("fetch: %w", f.err)
						}
						agent = f.agent

						// If the agent is no longer starting, stop following
						// logs because FetchLogs will keep streaming forever.
						// We do one last non-follow request to ensure we have
						// fetched all logs.
						if !agent.LifecycleState.Starting() {
							_ = logsCloser.Close()
							fetchedAgentWhileFollowing = nil

							logStream, logsCloser, err = opts.FetchLogs(ctx, agent.ID, lastLog.ID, false)
							if err != nil {
								return xerrors.Errorf("fetch workspace agent startup logs: %w", err)
							}
							// Logs are already primed, so we can call close.
							_ = logsCloser.Close()
						}
					case logs, ok := <-logStream:
						if !ok {
							return nil
						}
						for _, log := range logs {
							source, hasSource := logSources[log.SourceID]
							output := log.Output
							if hasSource && source.DisplayName != "" {
								output = source.DisplayName + ": " + output
							}
							sw.Log(log.CreatedAt, log.Level, output)
							lastLog = log
						}
					}
				}
			}()
			if err != nil {
				return err
			}

			for follow && agent.LifecycleState.Starting() {
				if agent, err = fetch(); err != nil {
					return xerrors.Errorf("fetch: %w", err)
				}
			}

			switch agent.LifecycleState {
			case codersdk.WorkspaceAgentLifecycleReady:
				sw.Complete(stage, safeDuration(sw, agent.ReadyAt, agent.StartedAt))
			case codersdk.WorkspaceAgentLifecycleStartTimeout:
				// Backwards compatibility: Avoid printing warning if
				// coderd is old and doesn't set ReadyAt for timeouts.
				if agent.ReadyAt == nil {
					sw.Fail(stage, 0)
				} else {
					sw.Fail(stage, safeDuration(sw, agent.ReadyAt, agent.StartedAt))
				}
				sw.Log(time.Time{}, codersdk.LogLevelWarn, "Warning: A startup script timed out and your workspace may be incomplete.")
			case codersdk.WorkspaceAgentLifecycleStartError:
				sw.Fail(stage, safeDuration(sw, agent.ReadyAt, agent.StartedAt))
				// Use zero time (omitted) to separate these from the startup logs.
				sw.Log(time.Time{}, codersdk.LogLevelWarn, "Warning: A startup script exited with an error and your workspace may be incomplete.")
				sw.Log(time.Time{}, codersdk.LogLevelWarn, troubleshootingMessage(agent, fmt.Sprintf("%s/templates#startup-script-exited-with-an-error", opts.DocsURL)))
			default:
				switch {
				case agent.LifecycleState.Starting():
					// Use zero time (omitted) to separate these from the startup logs.
					sw.Log(time.Time{}, codersdk.LogLevelWarn, "Notice: The startup scripts are still running and your workspace may be incomplete.")
					sw.Log(time.Time{}, codersdk.LogLevelWarn, troubleshootingMessage(agent, fmt.Sprintf("%s/templates#your-workspace-may-be-incomplete", opts.DocsURL)))
					// Note: We don't complete or fail the stage here, it's
					// intentionally left open to indicate this stage didn't
					// complete.
				case agent.LifecycleState.ShuttingDown():
					// We no longer know if the startup script failed or not,
					// but we need to tell the user something.
					sw.Complete(stage, safeDuration(sw, agent.ReadyAt, agent.StartedAt))
					return errAgentShuttingDown
				}
			}

			return nil

		case codersdk.WorkspaceAgentDisconnected:
			// If the agent was still starting during disconnect, we'll
			// show startup logs.
			showStartupLogs = agent.LifecycleState.Starting()

			stage := "The workspace agent lost connection"
			sw.Start(stage)
			sw.Log(time.Now(), codersdk.LogLevelWarn, "Wait for it to reconnect or restart your workspace.")
			sw.Log(time.Now(), codersdk.LogLevelWarn, troubleshootingMessage(agent, fmt.Sprintf("%s/templates#agent-connection-issues", opts.DocsURL)))

			disconnectedAt := agent.DisconnectedAt
			for agent.Status == codersdk.WorkspaceAgentDisconnected {
				if agent, err = fetch(); err != nil {
					return xerrors.Errorf("fetch: %w", err)
				}
			}
			sw.Complete(stage, safeDuration(sw, agent.LastConnectedAt, disconnectedAt))
		}
	}
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
		general = append(general, "❗ Direct connections are disabled locally, by `--disable-direct` or `CODER_DISABLE_DIRECT`")
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
