package agentmcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

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

	execTool := mcp.NewTool("workspace_exec",
		mcp.WithDescription(`Execute a command in a remote workspace on a given Coder deployment.`),
		// Required parameters.
		mcp.WithString("workspace_id", mcp.Description(`The workspace ID in which to execute the command in. The workspace must be running.`), mcp.Required()),
		mcp.WithString("command", mcp.Description(`The command to execute. Changing the working directory is not currently supported, so you may need to preface the command with 'cd /some/path && <my-command>'.`), mcp.Required()),
		// Optional parameters.
		mcp.WithString(`coder_url`, mcp.Description(`The URL of the Coder deployment. e.g. https://coder.example.com. Defaults to CODER_URL.`), mcp.DefaultString(os.Getenv("CODER_URL"))),
		mcp.WithString(`coder_session_token`, mcp.Description(`The session token for the Coder deployment. Defaults to CODER_SESSION_TOKEN.`), mcp.DefaultString(os.Getenv("CODER_SESSION_TOKEN"))),
	)

	srv.AddTool(execTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Set a reasonable upper limit on how long a command can take to run.
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 60*time.Second)
		defer timeoutCancel()
		args := request.Params.Arguments

		workspaceID, ok := args["workspace_id"].(string)
		if !ok {
			return nil, errors.New("workspace_id is required")
		}

		workspaceIDParsed, err := uuid.Parse(workspaceID)
		if err != nil {
			return nil, fmt.Errorf("workspace_id must be a valid UUID: %s", err.Error())
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
		ws, err := coderClient.Workspace(timeoutCtx, workspaceIDParsed)
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

		conn, err := workspacesdk.New(coderClient).AgentReconnectingPTY(timeoutCtx, workspacesdk.WorkspaceAgentReconnectingPTYOpts{
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
			return nil, xerrors.Errorf("failed to read from reconnecting PTY: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent(strings.TrimSpace(buf.String())),
			},
		}, nil
	})

	return server.ServeStdio(srv)
}
