package agentmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

func New(ctx context.Context, sdk *agentsdk.Client) error {
	srv := server.NewMCPServer(
		"Coder Agent",
		buildinfo.Version(),
		server.WithInstructions(`Report your progress when starting, working on, or completing a task.

You MUST report tasks when starting something new, or when you've completed a task.

You MUST report intermediate progress on a task if you've been working on it for a while.

Examples of sending a task:
- Working on a new part of the codebase.
- Starting on an issue (you should include the issue URL as "link").
- Opening a pull request (you should include the PR URL as "link").
- Completing a task (you should set "done" to true).
- Starting a new task (you should set "done" to false).
`),
	)

	taskTool := mcp.NewTool("report_task",
		mcp.WithDescription(`Report progress on a task.`),
		mcp.WithString("summary", mcp.Description(`A summary of your progress on a task.

Good Summaries:
- "Taking a look at the login page..."
- "Found a bug! Fixing it now..."
- "Investigating the GitHub Issue..."`), mcp.Required()),
		mcp.WithString("link", mcp.Description(`A relevant link to your work. e.g. GitHub issue link, pull request link, etc.`), mcp.Required()),
		mcp.WithBoolean("done", mcp.Description(`Whether the task the user requested is complete.`), mcp.Required()),
		mcp.WithString("emoji", mcp.Description(`A relevant emoji to your work.`), mcp.Required()),
	)

	srv.AddTool(taskTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments

		summary, ok := args["summary"].(string)
		if !ok {
			return nil, errors.New("summary is required")
		}

		link, ok := args["link"].(string)
		if !ok {
			return nil, errors.New("link is required")
		}

		emoji, ok := args["emoji"].(string)
		if !ok {
			return nil, errors.New("emoji is required")
		}

		done, ok := args["done"].(bool)
		if !ok {
			return nil, errors.New("done is required")
		}

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

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent("Thanks for reporting!"),
			},
		}, nil
	})

	whoamiTool := mcp.NewTool("coder_whoami",
		mcp.WithDescription(`Get information about the currently logged-in Coder user.`),
		mcp.WithString(`coder_url`, mcp.Description(`The URL of the Coder deployment. e.g. https://coder.example.com.`), mcp.Required()),
		mcp.WithString(`coder_session_token`, mcp.Description(`The session token for the Coder deployment.`), mcp.Required()),
	)

	srv.AddTool(whoamiTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments

		coderURL, ok := args["coder_url"].(string)
		if !ok {
			return nil, errors.New("coder_url is required, is CODER_URL set?")
		}

		coderSessionToken, ok := args["coder_session_token"].(string)
		if !ok {
			return nil, errors.New("coder_session_token is required, is CODER_SESSION_TOKEN set?")
		}

		coderURLParsed, err := url.Parse(coderURL)
		if err != nil {
			return nil, fmt.Errorf("invalid coder_url: %s", err.Error())
		}

		coderClient := codersdk.New(coderURLParsed)
		coderClient.SetSessionToken(coderSessionToken)

		me, err := coderClient.User(ctx, codersdk.Me)
		if err != nil {
			return nil, fmt.Errorf("Failed to fetch the current user: %s", err.Error())
		}

		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(me); err != nil {
			return nil, fmt.Errorf("Failed to encode the current user: %s", err.Error())
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent(strings.TrimSpace(buf.String())),
			},
		}, nil
	})

	execTool := mcp.NewTool("coder_workspace_exec",
		mcp.WithDescription(`Execute a command in a remote workspace on a given Coder deployment.`),
		// Required parameters.
		mcp.WithString("workspace", mcp.Description(`The workspace ID or name in which to execute the command in. The workspace must be running.`), mcp.Required()),
		mcp.WithString("command", mcp.Description(`The command to execute. Changing the working directory is not currently supported, so you may need to preface the command with 'cd /some/path && <my-command>'.`), mcp.Required()),
		// Optional parameters.
		mcp.WithString(`coder_url`, mcp.Description(`The URL of the Coder deployment. e.g. https://coder.example.com.`), mcp.Required()),
		mcp.WithString(`coder_session_token`, mcp.Description(`The session token for the Coder deployment.`), mcp.Required()),
	)

	srv.AddTool(execTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments

		workspaceID, ok := args["workspace"].(string)
		if !ok {
			return nil, errors.New("workspaceis required")
		}

		owner, workspaceName, err := splitNamedWorkspace(workspaceID)
		if err != nil {
			return nil, err
		}

		command, ok := args["command"].(string)
		if !ok {
			return nil, errors.New("command is required")
		}

		coderURL, ok := args["coder_url"].(string)
		if !ok {
			return nil, errors.New("coder_url is required, is CODER_URL set?")
		}

		coderSessionToken, ok := args["coder_session_token"].(string)
		if !ok {
			return nil, errors.New("coder_session_token is required, is CODER_SESSION_TOKEN set?")
		}

		coderURLParsed, err := url.Parse(coderURL)
		if err != nil {
			return nil, fmt.Errorf("invalid coder_url: %s", err.Error())
		}

		coderClient := codersdk.New(coderURLParsed)
		coderClient.SetSessionToken(coderSessionToken)

		// Attempt to fetch the workspace.
		ws, err := coderClient.WorkspaceByOwnerAndName(ctx, owner, workspaceName, codersdk.WorkspaceOptions{})
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
			return nil, xerrors.Errorf("no connected agents for workspace %s", workspaceID)
		}

		conn, err := workspacesdk.New(coderClient).AgentReconnectingPTY(ctx, workspacesdk.WorkspaceAgentReconnectingPTYOpts{
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
			if errors.Is(err, io.EOF) {
				// EOF is expected when the connection is closed.
				// We can ignore this error.
			} else {
				return nil, xerrors.Errorf("failed to read from reconnecting PTY: %w", err)
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent(strings.TrimSpace(buf.String())),
			},
		}, nil
	})

	listWorkspacesTool := mcp.NewTool("coder_list_workspaces",
		mcp.WithDescription(`List workspaces on a given Coder deployment owned by the current user.`),
		mcp.WithString(`coder_url`, mcp.Description(`The URL of the Coder deployment. e.g. https://coder.example.com. Defaults to CODER_URL.`), mcp.Required()),
		mcp.WithString(`coder_session_token`, mcp.Description(`The session token for the Coder deployment. Defaults to CODER_SESSION_TOKEN.`), mcp.Required()),
		mcp.WithString(`owner`, mcp.Description(`The owner of the workspaces to list. Defaults to the current user.`), mcp.DefaultString(codersdk.Me)),
		mcp.WithNumber(`offset`, mcp.Description(`The offset to start listing workspaces from. Defaults to 0.`), mcp.DefaultNumber(0)),
		mcp.WithNumber(`limit`, mcp.Description(`The maximum number of workspaces to list. Defaults to 10.`), mcp.DefaultNumber(10)),
	)

	srv.AddTool(listWorkspacesTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments

		coderURL, ok := args["coder_url"].(string)
		if !ok {
			return nil, errors.New("coder_url is required, is CODER_URL set?")
		}

		coderSessionToken, ok := args["coder_session_token"].(string)
		if !ok {
			return nil, errors.New("coder_session_token is required, is CODER_SESSION_TOKEN set?")
		}

		coderURLParsed, err := url.Parse(coderURL)
		if err != nil {
			return nil, fmt.Errorf("invalid coder_url: %s", err.Error())
		}

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

		coderClient := codersdk.New(coderURLParsed)
		coderClient.SetSessionToken(coderSessionToken)

		workspaces, err := coderClient.Workspaces(ctx, codersdk.WorkspaceFilter{
			Owner:  owner,
			Offset: offset,
			Limit:  limit,
		})

		// Encode it as JSON. TODO: It might be nicer for the agent to have a tabulated response.
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		if err := enc.Encode(workspaces); err != nil {
			return nil, fmt.Errorf("failed to encode workspaces: %s", err.Error())
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent(strings.TrimSpace(buf.String())),
			},
		}, nil
	})

	return server.ServeStdio(srv)
}

// COPYPASTA: from cli/root.go
func splitNamedWorkspace(identifier string) (owner string, workspaceName string, err error) {
	parts := strings.Split(identifier, "/")

	switch len(parts) {
	case 1:
		owner = codersdk.Me
		workspaceName = parts[0]
	case 2:
		owner = parts[0]
		workspaceName = parts[1]
	default:
		return "", "", fmt.Errorf("invalid workspace name: %q should be in the format <workspace> or <owner>/<workspace>", identifier)
	}
	return owner, workspaceName, nil
}
