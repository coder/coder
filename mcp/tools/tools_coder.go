package mcptools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_report_task", "arguments": {"summary": "I'm working on the login page.", "link": "https://github.com/coder/coder/pull/1234", "emoji": "🔍", "done": false}}}
func handleCoderReportTask(deps ToolDeps) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}

		args := request.Params.Arguments

		summary, ok := args["summary"].(string)
		if !ok {
			return nil, xerrors.New("summary is required")
		}

		link, ok := args["link"].(string)
		if !ok {
			return nil, xerrors.New("link is required")
		}

		emoji, ok := args["emoji"].(string)
		if !ok {
			return nil, xerrors.New("emoji is required")
		}

		done, ok := args["done"].(bool)
		if !ok {
			return nil, xerrors.New("done is required")
		}

		// TODO: Waiting on support for tasks.
		deps.Logger.Info(ctx, "report task tool called", slog.F("summary", summary), slog.F("link", link), slog.F("done", done), slog.F("emoji", emoji))
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

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_list_workspaces", "arguments": {"owner": "me", "offset": 0, "limit": 10}}}
func handleCoderListWorkspaces(deps ToolDeps) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}
		args := request.Params.Arguments

		owner, ok := args["owner"].(string)
		if !ok {
			owner = codersdk.Me
		}

		offset, ok := args["offset"].(int)
		if !ok || offset < 0 {
			offset = 0
		}
		limit, ok := args["limit"].(int)
		if !ok || limit <= 0 {
			limit = 10
		}

		workspaces, err := deps.Client.Workspaces(ctx, codersdk.WorkspaceFilter{
			Owner:  owner,
			Offset: offset,
			Limit:  limit,
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

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_get_workspace", "arguments": {"workspace": "dev"}}}
func handleCoderGetWorkspace(deps ToolDeps) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}
		args := request.Params.Arguments

		wsArg, ok := args["workspace"].(string)
		if !ok {
			return nil, xerrors.New("workspace is required")
		}

		workspace, err := getWorkspaceByIDOrOwnerName(ctx, deps.Client, wsArg)
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

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_workspace_exec", "arguments": {"workspace": "dev", "command": "ps -ef"}}}
func handleCoderWorkspaceExec(deps ToolDeps) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}
		args := request.Params.Arguments

		wsArg, ok := args["workspace"].(string)
		if !ok {
			return nil, xerrors.New("workspace is required")
		}

		command, ok := args["command"].(string)
		if !ok {
			return nil, xerrors.New("command is required")
		}

		// Validate the command if allowed commands are specified
		allowed, err := IsCommandAllowed(command, deps.AllowedExecCommands)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, xerrors.Errorf("command not allowed: %s", command)
		}

		// Attempt to fetch the workspace. We may get a UUID or a name, so try to
		// handle both.
		ws, err := getWorkspaceByIDOrOwnerName(ctx, deps.Client, wsArg)
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

		conn, err := workspacesdk.New(deps.Client).AgentReconnectingPTY(ctx, workspacesdk.WorkspaceAgentReconnectingPTYOpts{
			AgentID:     agt.ID,
			Reconnect:   uuid.New(),
			Width:       80,
			Height:      24,
			Command:     command,
			BackendType: "buffered", // the screen backend is annoying to use here.
		})
		if err != nil {
			return nil, xerrors.Errorf("failed to open reconnecting PTY: %w", err)
		}
		defer conn.Close()

		var buf bytes.Buffer
		if _, err := io.Copy(&buf, conn); err != nil {
			// EOF is expected when the connection is closed.
			// We can ignore this error.
			if !errors.Is(err, io.EOF) {
				return nil, xerrors.Errorf("failed to read from reconnecting PTY: %w", err)
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent(strings.TrimSpace(buf.String())),
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

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_start_workspace", "arguments": {"workspace": "dev"}}}
func handleCoderStartWorkspace(deps ToolDeps) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}

		args := request.Params.Arguments

		wsArg, ok := args["workspace"].(string)
		if !ok {
			return nil, xerrors.New("workspace is required")
		}

		workspace, err := getWorkspaceByIDOrOwnerName(ctx, deps.Client, wsArg)
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch workspace: %w", err)
		}

		switch workspace.LatestBuild.Status {
		case codersdk.WorkspaceStatusPending, codersdk.WorkspaceStatusStarting, codersdk.WorkspaceStatusRunning, codersdk.WorkspaceStatusCanceling:
			return nil, xerrors.Errorf("workspace is %s", workspace.LatestBuild.Status)
		}

		wb, err := deps.Client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionStart,
		})
		if err != nil {
			return nil, xerrors.Errorf("failed to start workspace: %w", err)
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

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_stop_workspace", "arguments": {"workspace": "dev"}}}
func handleCoderStopWorkspace(deps ToolDeps) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.Client == nil {
			return nil, xerrors.New("developer error: client is required")
		}

		args := request.Params.Arguments

		wsArg, ok := args["workspace"].(string)
		if !ok {
			return nil, xerrors.New("workspace is required")
		}

		workspace, err := getWorkspaceByIDOrOwnerName(ctx, deps.Client, wsArg)
		if err != nil {
			return nil, xerrors.Errorf("failed to fetch workspace: %w", err)
		}

		switch workspace.LatestBuild.Status {
		case codersdk.WorkspaceStatusPending, codersdk.WorkspaceStatusStopping, codersdk.WorkspaceStatusStopped, codersdk.WorkspaceStatusCanceling:
			return nil, xerrors.Errorf("workspace is %s", workspace.LatestBuild.Status)
		}

		wb, err := deps.Client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
			Transition: codersdk.WorkspaceTransitionStop,
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
