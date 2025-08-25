package cli

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
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

// createStream creates a new immortal stream
func (c *immortalStreamClient) createStream(ctx context.Context, port int) (*codersdk.ImmortalStream, error) {
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
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "immortal-stream",
		Short: "Manage immortal streams in workspaces",
		Long:  "Immortal streams provide persistent TCP connections to workspace services that automatically reconnect when interrupted.",
		Middleware: serpent.Chain(
			r.InitClient(client),
		),
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
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "list <workspace-name>",
		Short: "List active immortal streams in a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			workspaceName := inv.Args[0]

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
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "delete <workspace-name> <immortal-stream-name>",
		Short: "Delete an active immortal stream",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(2),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			workspaceName := inv.Args[0]
			streamName := inv.Args[1]

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
