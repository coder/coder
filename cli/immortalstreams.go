package cli

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/immortalstreams/backedpipe"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/serpent"
)

// immortalStreamClient provides methods to interact with immortal streams API
// This uses the main codersdk.Client to make server-proxied requests to agents
type immortalStreamClient struct {
	client  *codersdk.Client
	agentID uuid.UUID
	logger  slog.Logger
}

// newImmortalStreamClient creates a new client for immortal streams
func newImmortalStreamClient(client *codersdk.Client, agentID uuid.UUID, logger slog.Logger) *immortalStreamClient {
	return &immortalStreamClient{
		client:  client,
		agentID: agentID,
		logger:  logger,
	}
}

// ImmortalDialOptions control dialing behavior for immortal streams.
type ImmortalDialOptions struct {
	Enabled  bool
	Fallback bool
	// TargetPort is the agent TCP port to connect to when using an immortal stream (e.g., 1 for SSH).
	TargetPort uint16
	// CoderConnectHost, if set, is the hostname to use when Coder Connect is running (e.g. agent.workspace.owner.suffix).
	CoderConnectHost string
}

// ImmortalConnResult describes the connection and (if used) associated immortal stream resources.
type ImmortalConnResult struct {
	Conn         net.Conn
	StreamClient *immortalStreamClient
	StreamID     *uuid.UUID
	UsedImmortal bool
}

// DialImmortalOrFallback creates an immortal stream and connects to it, or falls back
// to a provided dialer if disabled or creation fails and fallback is allowed.
func DialImmortalOrFallback(
	ctx context.Context,
	agentConn workspacesdk.AgentConn,
	client *codersdk.Client,
	agentID uuid.UUID,
	logger slog.Logger,
	ops ImmortalDialOptions,
	fallbackDial func(context.Context) (net.Conn, error),
) (ImmortalConnResult, error) {
	if !ops.Enabled {
		c, err := fallbackDial(ctx)
		if err != nil {
			return ImmortalConnResult{}, err
		}
		return ImmortalConnResult{Conn: c, UsedImmortal: false}, nil
	}

	streamClient := newImmortalStreamClient(client, agentID, logger)
	stream, err := streamClient.createStream(ctx, ops.TargetPort)
	if err != nil {
		logger.Error(ctx, "failed to create immortal stream", slog.Error(err), slog.F("agent_id", agentID), slog.F("target_port", ops.TargetPort), slog.F("immortal_fallback_enabled", ops.Fallback))
		// Allow fallback for common infra/transient errors in addition to explicit limits.
		lower := strings.ToLower(err.Error())
		// Be lenient on message formatting/casing from server
		tooManyErr := strings.Contains(lower, "too many immortal stream")
		connRefused := strings.Contains(err.Error(), "The connection was refused") || strings.Contains(lower, "connection refused")
		timeoutErr := strings.Contains(lower, "context deadline exceeded") || strings.Contains(lower, "timeout")
		unreachableErr := strings.Contains(lower, "not reachable")
		shouldFallback := ops.Fallback && (tooManyErr || connRefused || timeoutErr || unreachableErr)
		if !shouldFallback {
			return ImmortalConnResult{}, xerrors.Errorf("create immortal stream: %w", err)
		}
		switch {
		case tooManyErr:
			logger.Warn(ctx, "too many immortal streams, falling back to regular connection", slog.F("max_streams", "32"), slog.F("target_port", ops.TargetPort))
		case connRefused:
			logger.Warn(ctx, "service not available, falling back to regular connection", slog.F("reason", "connection_refused"), slog.F("target_port", ops.TargetPort))
		case timeoutErr:
			logger.Warn(ctx, "agent HTTP API timed out, falling back to regular connection", slog.F("reason", "context_deadline_exceeded"), slog.F("target_port", ops.TargetPort))
		case unreachableErr:
			logger.Warn(ctx, "agent unreachable for immortal stream, falling back to regular connection", slog.F("target_port", ops.TargetPort))
		}
		c, derr := fallbackDial(ctx)
		if derr != nil {
			return ImmortalConnResult{}, xerrors.Errorf("fallback dial: %w", derr)
		}
		return ImmortalConnResult{Conn: c, UsedImmortal: false}, nil
	}

	// connect to the stream using the agent tailnet connection
	logger.Info(ctx, "immortal: creating reconnector", slog.F("agent_id", agentID), slog.F("stream_id", stream.ID))
	reconnector := newClientStreamReconnector(ctx, agentConn, client, agentID, stream.ID, logger, ops.CoderConnectHost)
	logger.Info(ctx, "immortal: created reconnector")
	pipe := backedpipe.NewBackedPipe(ctx, reconnector)
	// Ensure a 404 on upgrade terminates the pipe immediately.
	reconnector.onPermanentFailure = func() {
		_ = pipe.Close()
	}
	logger.Info(ctx, "immortal: connecting backed pipe")
	if err := pipe.Connect(); err != nil {
		_ = streamClient.deleteStream(ctx, stream.ID)
		return ImmortalConnResult{}, xerrors.Errorf("connect to immortal stream: %w", err)
	}
	logger.Info(ctx, "immortal: backed pipe connected")

	conn := &immortalBackedConn{ctx: ctx, cancel: func() {}, pipe: pipe, logger: logger}
	conn.startSupervisor()

	streamID := stream.ID
	return ImmortalConnResult{
		Conn:         conn,
		StreamClient: streamClient,
		StreamID:     &streamID,
		UsedImmortal: true,
	}, nil
}

// createStream creates a new immortal stream
func (c *immortalStreamClient) createStream(ctx context.Context, port uint16) (*codersdk.ImmortalStream, error) {
	stream, err := c.client.WorkspaceAgentCreateImmortalStream(ctx, c.agentID, codersdk.CreateImmortalStreamRequest{
		TCPPort: port,
	})
	if err != nil {
		return nil, err
	}
	return &stream, nil
}

// listStreams lists all immortal streams
func (c *immortalStreamClient) listStreams(ctx context.Context) ([]codersdk.ImmortalStream, error) {
	return c.client.WorkspaceAgentImmortalStreams(ctx, c.agentID)
}

// deleteStream deletes an immortal stream
func (c *immortalStreamClient) deleteStream(ctx context.Context, streamID uuid.UUID) error {
	return c.client.WorkspaceAgentDeleteImmortalStream(ctx, c.agentID, streamID)
}

// CLI Commands

func (r *RootCmd) immortalStreamCmd() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "immortal-stream",
		Short: "Manage immortal streams in workspaces",
		Long:  "Immortal streams provide persistent TCP connections to workspace services that automatically reconnect when interrupted.",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.immortalStreamListCmd(),
			r.immortalStreamDeleteCmd(),
		},
	}
	return cmd
}

func (r *RootCmd) immortalStreamListCmd() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "list <workspace-name>",
		Short: "List active immortal streams in a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			workspaceName := inv.Args[0]

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			workspace, workspaceAgent, _, err := GetWorkspaceAndAgent(ctx, inv, client, false, workspaceName)
			if err != nil {
				return err
			}

			if workspace.LatestBuild.Transition != codersdk.WorkspaceTransitionStart {
				return xerrors.New("workspace must be running to list immortal streams")
			}

			// Create immortal stream client
			// Note: We don't need to dial the agent for management operations
			// as these go through the server's proxy endpoints
			streamClient := newImmortalStreamClient(client, workspaceAgent.ID, inv.Logger)
			streams, err := streamClient.listStreams(ctx)
			if err != nil {
				return xerrors.Errorf("list immortal streams: %w", err)
			}

			if len(streams) == 0 {
				cliui.Info(inv.Stderr, "No active immortal streams found.")
				return nil
			}

			// Display the streams in a table
			displayImmortalStreams(inv, streams)
			return nil
		},
	}
	return cmd
}

func (r *RootCmd) immortalStreamDeleteCmd() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "delete <workspace-name> <immortal-stream-name>",
		Short: "Delete an active immortal stream",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(2),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			workspaceName := inv.Args[0]
			streamName := inv.Args[1]

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			workspace, workspaceAgent, _, err := GetWorkspaceAndAgent(ctx, inv, client, false, workspaceName)
			if err != nil {
				return err
			}

			if workspace.LatestBuild.Transition != codersdk.WorkspaceTransitionStart {
				return xerrors.New("workspace must be running to delete immortal streams")
			}

			// Create immortal stream client
			streamClient := newImmortalStreamClient(client, workspaceAgent.ID, inv.Logger)
			streams, err := streamClient.listStreams(ctx)
			if err != nil {
				return xerrors.Errorf("list immortal streams: %w", err)
			}

			var targetStream *codersdk.ImmortalStream
			for _, stream := range streams {
				if stream.Name == streamName {
					targetStream = &stream
					break
				}
			}

			if targetStream == nil {
				return xerrors.Errorf("immortal stream %q not found", streamName)
			}

			// Delete the stream
			err = streamClient.deleteStream(ctx, targetStream.ID)
			if err != nil {
				return xerrors.Errorf("delete immortal stream: %w", err)
			}

			cliui.Info(inv.Stderr, fmt.Sprintf("Deleted immortal stream %q (ID: %s)", streamName, targetStream.ID))
			return nil
		},
	}
	return cmd
}

func displayImmortalStreams(inv *serpent.Invocation, streams []codersdk.ImmortalStream) {
	_, _ = fmt.Fprintf(inv.Stderr, "Active Immortal Streams:\n\n")
	_, _ = fmt.Fprintf(inv.Stderr, "%-20s %-6s %-20s %-20s\n", "NAME", "PORT", "CREATED", "LAST CONNECTED")
	_, _ = fmt.Fprintf(inv.Stderr, "%-20s %-6s %-20s %-20s\n", "----", "----", "-------", "--------------")

	for _, stream := range streams {
		createdTime := stream.CreatedAt.Format("2006-01-02 15:04:05")
		lastConnTime := stream.LastConnectionAt.Format("2006-01-02 15:04:05")

		_, _ = fmt.Fprintf(inv.Stderr, "%-20s %-6d %-20s %-20s\n",
			stream.Name, stream.TCPPort, createdTime, lastConnTime)
	}
	_, _ = fmt.Fprintf(inv.Stderr, "\n")
}
