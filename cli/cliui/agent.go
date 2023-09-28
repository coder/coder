package cliui

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

var errAgentShuttingDown = xerrors.New("agent is shutting down")

type AgentOptions struct {
	FetchInterval time.Duration
	Fetch         func(ctx context.Context, agentID uuid.UUID) (codersdk.WorkspaceAgent, error)
	FetchLogs     func(ctx context.Context, agentID uuid.UUID, after int64, follow bool) (<-chan []codersdk.WorkspaceAgentLog, io.Closer, error)
	Wait          bool // If true, wait for the agent to be ready (startup script).
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
				sw.Log(now, codersdk.LogLevelInfo, troubleshootingMessage(agent, "https://coder.com/docs/v2/latest/templates#agent-connection-issues"))
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
			follow := opts.Wait
			if !follow {
				stage += " (non-blocking)"
			}
			sw.Start(stage)

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
				sw.Complete(stage, agent.ReadyAt.Sub(*agent.StartedAt))
			case codersdk.WorkspaceAgentLifecycleStartTimeout:
				sw.Fail(stage, 0)
				sw.Log(time.Time{}, codersdk.LogLevelWarn, "Warning: A startup script timed out and your workspace may be incomplete.")
			case codersdk.WorkspaceAgentLifecycleStartError:
				sw.Fail(stage, agent.ReadyAt.Sub(*agent.StartedAt))
				// Use zero time (omitted) to separate these from the startup logs.
				sw.Log(time.Time{}, codersdk.LogLevelWarn, "Warning: A startup script exited with an error and your workspace may be incomplete.")
				sw.Log(time.Time{}, codersdk.LogLevelWarn, troubleshootingMessage(agent, "https://coder.com/docs/v2/latest/templates#startup-script-exited-with-an-error"))
			default:
				switch {
				case agent.LifecycleState.Starting():
					// Use zero time (omitted) to separate these from the startup logs.
					sw.Log(time.Time{}, codersdk.LogLevelWarn, "Notice: The startup scripts are still running and your workspace may be incomplete.")
					sw.Log(time.Time{}, codersdk.LogLevelWarn, troubleshootingMessage(agent, "https://coder.com/docs/v2/latest/templates#your-workspace-may-be-incomplete"))
					// Note: We don't complete or fail the stage here, it's
					// intentionally left open to indicate this stage didn't
					// complete.
				case agent.LifecycleState.ShuttingDown():
					// We no longer know if the startup script failed or not,
					// but we need to tell the user something.
					sw.Complete(stage, agent.ReadyAt.Sub(*agent.StartedAt))
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
			sw.Log(time.Now(), codersdk.LogLevelWarn, troubleshootingMessage(agent, "https://coder.com/docs/v2/latest/templates#agent-connection-issues"))

			disconnectedAt := *agent.DisconnectedAt
			for agent.Status == codersdk.WorkspaceAgentDisconnected {
				if agent, err = fetch(); err != nil {
					return xerrors.Errorf("fetch: %w", err)
				}
			}
			sw.Complete(stage, agent.LastConnectedAt.Sub(disconnectedAt))
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

type closeFunc func() error

func (c closeFunc) Close() error {
	return c()
}
