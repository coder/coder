package mcptools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"golang.org/x/xerrors"
)

type ToolAdder interface {
	AddTool(tool mcp.Tool, handler mcpserver.ToolHandlerFunc)
}

func RegisterCoderWhoami(ta ToolAdder) {
	ta.AddTool(toolCoderWhoami, handleCoderWhoami)
}

func RegisterCoderListWorkspaces(ta ToolAdder) {
	ta.AddTool(toolCoderListWorkspaces, handleCoderListWorkspaces)
}

func RegisterCoderWorkspaceExec(ta ToolAdder) {
	ta.AddTool(toolCoderWorkspaceExec, handleCoderWorkspaceExec)
}

var (
	toolCoderWhoami = mcp.NewTool("coder_whoami",
		mcp.WithDescription(`Get information about the currently logged-in Coder user.`),
		mcp.WithString(`coder_url`, mcp.Description(`The URL of the Coder deployment. e.g. https://coder.example.com. This is accessible via CODER_URL or CODER_AGENT_URL.`), mcp.Required()),
		mcp.WithString(`coder_session_token`, mcp.Description(`The session token for the Coder deployment. This is accessible via CODER_SESSION_TOKEN.`), mcp.Required()),
	)
	toolCoderListWorkspaces = mcp.NewTool("coder_list_workspaces",
		mcp.WithDescription(`List workspaces on a given Coder deployment owned by the current user.`),
		mcp.WithString(`owner`, mcp.Description(`The owner of the workspaces to list. Defaults to the current user.`), mcp.DefaultString(codersdk.Me)),
		mcp.WithNumber(`offset`, mcp.Description(`The offset to start listing workspaces from. Defaults to 0.`), mcp.DefaultNumber(0)),
		mcp.WithNumber(`limit`, mcp.Description(`The maximum number of workspaces to list. Defaults to 10.`), mcp.DefaultNumber(10)),
		mcp.WithString(`coder_url`, mcp.Description(`The URL of the Coder deployment. e.g. https://coder.example.com. This is accessible via CODER_URL or CODER_AGENT_URL.`), mcp.Required()),
		mcp.WithString(`coder_session_token`, mcp.Description(`The session token for the Coder deployment. This is accessible via CODER_SESSION_TOKEN.`), mcp.Required()),
	)
	toolCoderWorkspaceExec = mcp.NewTool("coder_workspace_exec",
		mcp.WithDescription(`Execute a command in a remote workspace on a given Coder deployment.`),
		// Required parameters.
		mcp.WithString("workspace", mcp.Description(`The workspace ID or name in which to execute the command in. The workspace must be running.`), mcp.Required()),
		mcp.WithString("command", mcp.Description(`The command to execute. Changing the working directory is not currently supported, so you may need to preface the command with 'cd /some/path && <my-command>'.`), mcp.Required()),
		mcp.WithString(`coder_url`, mcp.Description(`The URL of the Coder deployment. e.g. https://coder.example.com. This is accessible via CODER_URL or CODER_AGENT_URL.`), mcp.Required()),
		mcp.WithString(`coder_session_token`, mcp.Description(`The session token for the Coder deployment. This is accessible via CODER_SESSION_TOKEN.`), mcp.Required()),
	)
)

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_whoami", "arguments": {"coder_url": "http://localhost:3000", "coder_session_token": "REDACTED"}}}
func handleCoderWhoami(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
}

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_list_workspaces", "arguments": {"owner": "me", "offset": 0, "limit": 10, "coder_url": "http://localhost:3000", "coder_session_token": "REDACTED"}}}
func handleCoderListWorkspaces(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
}

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_workspace_exec", "arguments": {"workspace": "dev", "command": "ps -ef", "coder_url": "http://localhost:3000", "coder_session_token": "REDACTED"}}}
func handleCoderWorkspaceExec(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
