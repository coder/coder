package mcptools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type handleCoderReportTaskArgs struct {
	Summary string `json:"summary"`
	Link    string `json:"link"`
	Emoji   string `json:"emoji"`
	Done    bool   `json:"done"`
}

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_report_task", "arguments": {"summary": "I'm working on the login page.", "link": "https://github.com/coder/coder/pull/1234", "emoji": "🔍", "done": false}}}
func handleCoderReportTask(deps ToolDeps) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}

		// Convert the request parameters to a json.RawMessage so we can unmarshal
		// them into the correct struct.
		args, err := unmarshalArgs[handleCoderReportTaskArgs](request.Params.Arguments)
		if err != nil {
			return nil, xerrors.Errorf("failed to unmarshal arguments: %w", err)
		}

		// TODO: Waiting on support for tasks.
		deps.Logger.Info(ctx, "report task tool called", slog.F("summary", args.Summary), slog.F("link", args.Link), slog.F("done", args.Done), slog.F("emoji", args.Emoji))
		/*
			err := sdk.PostTask(ctx, agentsdk.PostTaskRequest{
				Reporter:   "claude",
				Summary:    summary,
				URL:        link,
				Completion: done,
				Icon:       emoji,
			})
			if err != nil {
				return nil, err
			}
		*/

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent("Thanks for reporting!"),
			},
		}, nil
	}
}

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_whoami", "arguments": {}}}
func handleCoderWhoami(deps ToolDeps) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}
		me, err := deps.Client.User(ctx, codersdk.Me)
		if err != nil {
			return nil, xerrors.Errorf("Failed to fetch the current user: %s", err.Error())
		}

		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(me); err != nil {
			return nil, xerrors.Errorf("Failed to encode the current user: %s", err.Error())
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent(strings.TrimSpace(buf.String())),
			},
		}, nil
	}
}

type handleCoderListWorkspacesArgs struct {
	Owner  string `json:"owner"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_list_workspaces", "arguments": {"owner": "me", "offset": 0, "limit": 10}}}
func handleCoderListWorkspaces(deps ToolDeps) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}
		args, err := unmarshalArgs[handleCoderListWorkspacesArgs](request.Params.Arguments)
		if err != nil {
			return nil, xerrors.Errorf("failed to unmarshal arguments: %w", err)
		}

		workspaces, err := deps.Client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Owner:  args.Owner,
			Offset: args.Offset,
			Limit:  args.Limit,
		})
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch workspaces: %w", err)
		}

		// Encode it as JSON. TODO: It might be nicer for the agent to have a tabulated response.
		data, err := json.Marshal(workspaces)
		if err != nil {
			return nil, xerrors.Errorf("failed to encode workspaces: %s", err.Error())
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent(string(data)),
			},
		}, nil
	}
}

type handleCoderGetWorkspaceArgs struct {
	Workspace string `json:"workspace"`
}

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_get_workspace", "arguments": {"workspace": "dev"}}}
func handleCoderGetWorkspace(deps ToolDeps) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}
		args, err := unmarshalArgs[handleCoderGetWorkspaceArgs](request.Params.Arguments)
		if err != nil {
			return nil, xerrors.Errorf("failed to unmarshal arguments: %w", err)
		}

		workspace, err := getWorkspaceByIDOrOwnerName(ctx, deps.Client, args.Workspace)
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch workspace: %w", err)
		}

		workspaceJSON, err := json.Marshal(workspace)
		if err != nil {
			return nil, xerrors.Errorf("failed to encode workspace: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent(string(workspaceJSON)),
			},
		}, nil
	}
}

type handleCoderWorkspaceExecArgs struct {
	Workspace string `json:"workspace"`
	Command   string `json:"command"`
}

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_workspace_exec", "arguments": {"workspace": "dev", "command": "ps -ef"}}}
func handleCoderWorkspaceExec(deps ToolDeps) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}
		args, err := unmarshalArgs[handleCoderWorkspaceExecArgs](request.Params.Arguments)
		if err != nil {
			return nil, xerrors.Errorf("failed to unmarshal arguments: %w", err)
		}

		// Attempt to fetch the workspace. We may get a UUID or a name, so try to
		// handle both.
		ws, err := getWorkspaceByIDOrOwnerName(ctx, deps.Client, args.Workspace)
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch workspace: %w", err)
		}

		// Ensure the workspace is started.
		// Select the first agent of the workspace.
		var agt *codersdk.WorkspaceAgent
		for _, r := range ws.LatestBuild.Resources {
			for _, a := range r.Agents {
				if a.Status != codersdk.WorkspaceAgentConnected {
					continue
				}
				agt = ptr.Ref(a)
				break
			}
		}
		if agt == nil {
			return nil, xerrors.Errorf("no connected agents for workspace %s", ws.ID)
		}

		startedAt := time.Now()
		conn, err := workspacesdk.New(deps.Client).AgentReconnectingPTY(ctx, workspacesdk.WorkspaceAgentReconnectingPTYOpts{
			AgentID:     agt.ID,
			Reconnect:   uuid.New(),
			Width:       80,
			Height:      24,
			Command:     args.Command,
			BackendType: "buffered", // the screen backend is annoying to use here.
		})
		if err != nil {
			return nil, xerrors.Errorf("failed to open reconnecting PTY: %w", err)
		}
		defer conn.Close()
		connectedAt := time.Now()

		var buf bytes.Buffer
		if _, err := io.Copy(&buf, conn); err != nil {
			// EOF is expected when the connection is closed.
			// We can ignore this error.
			if !errors.Is(err, io.EOF) {
				return nil, xerrors.Errorf("failed to read from reconnecting PTY: %w", err)
			}
		}
		completedAt := time.Now()
		connectionTime := connectedAt.Sub(startedAt)
		executionTime := completedAt.Sub(connectedAt)

		resp := map[string]string{
			"connection_time": connectionTime.String(),
			"execution_time":  executionTime.String(),
			"output":          buf.String(),
		}
		respJSON, err := json.Marshal(resp)
		if err != nil {
			return nil, xerrors.Errorf("failed to encode workspace build: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent(string(respJSON)),
			},
		}, nil
	}
}

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_list_templates", "arguments": {}}}
func handleCoderListTemplates(deps ToolDeps) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}
		templates, err := deps.Client.Templates(ctx, codersdk.TemplateFilter{})
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch templates: %w", err)
		}

		templateJSON, err := json.Marshal(templates)
		if err != nil {
			return nil, xerrors.Errorf("failed to encode templates: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent(string(templateJSON)),
			},
		}, nil
	}
}

type handleCoderWorkspaceTransitionArgs struct {
	Workspace  string `json:"workspace"`
	Transition string `json:"transition"`
}

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name":
// "coder_workspace_transition", "arguments": {"workspace": "dev", "transition": "stop"}}}
func handleCoderWorkspaceTransition(deps ToolDeps) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}
		args, err := unmarshalArgs[handleCoderWorkspaceTransitionArgs](request.Params.Arguments)
		if err != nil {
			return nil, xerrors.Errorf("failed to unmarshal arguments: %w", err)
		}

		workspace, err := getWorkspaceByIDOrOwnerName(ctx, deps.Client, args.Workspace)
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch workspace: %w", err)
		}

		wsTransition := codersdk.WorkspaceTransition(args.Transition)
		switch wsTransition {
		case codersdk.WorkspaceTransitionStart:
		case codersdk.WorkspaceTransitionStop:
		default:
			return nil, xerrors.New("invalid transition")
		}

		// We're not going to check the workspace status here as it is checked on the
		// server side.
		wb, err := deps.Client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: wsTransition,
		})
		if err != nil {
			return nil, xerrors.Errorf("failed to stop workspace: %w", err)
		}

		resp := map[string]any{"status": wb.Status, "transition": wb.Transition}
		respJSON, err := json.Marshal(resp)
		if err != nil {
			return nil, xerrors.Errorf("failed to encode workspace build: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent(string(respJSON)),
			},
		}, nil
	}
}

func getWorkspaceByIDOrOwnerName(ctx context.Context, client *codersdk.Client, identifier string) (codersdk.Workspace, error) {
	if wsid, err := uuid.Parse(identifier); err == nil {
		return client.Workspace(ctx, wsid)
	}
	return client.WorkspaceByOwnerAndName(ctx, codersdk.Me, identifier, codersdk.WorkspaceOptions{})
}

// unmarshalArgs is a helper function to convert the map[string]any we get from
// the MCP server into a typed struct. It does this by marshaling and unmarshalling
// the arguments.
func unmarshalArgs[T any](args map[string]interface{}) (t T, err error) {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return t, xerrors.Errorf("failed to marshal arguments: %w", err)
	}
	if err := json.Unmarshal(argsJSON, &t); err != nil {
		return t, xerrors.Errorf("failed to unmarshal arguments: %w", err)
	}
	return t, nil
}
