package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// allTools is the list of all available tools. When adding a new tool,
// make sure to update this list.
var allTools = ToolRegistry{
	{
		Tool: mcp.NewTool("coder_report_task",
			mcp.WithDescription(`Report progress on a user task in Coder.
Use this tool to keep the user informed about your progress with their request.
For long-running operations, call this periodically to provide status updates.
This is especially useful when performing multi-step operations like workspace creation or deployment.`),
			mcp.WithString("summary", mcp.Description(`A concise summary of your current progress on the task.

Good Summaries:
- "Taking a look at the login page..."
- "Found a bug! Fixing it now..."
- "Investigating the GitHub Issue..."
- "Waiting for workspace to start (1/3 resources ready)"
- "Downloading template files from repository"`), mcp.Required()),
			mcp.WithString("link", mcp.Description(`A relevant URL related to your work, such as:
- GitHub issue link
- Pull request URL
- Documentation reference
- Workspace URL
Use complete URLs (including https://) when possible.`), mcp.Required()),
			mcp.WithString("emoji", mcp.Description(`A relevant emoji that visually represents the current status:
- 🔍 for investigating/searching
- 🚀 for deploying/starting
- 🐛 for debugging
- ✅ for completion
- ⏳ for waiting
Choose an emoji that helps the user understand the current phase at a glance.`), mcp.Required()),
			mcp.WithBoolean("done", mcp.Description(`Whether the overall task the user requested is complete.
Set to true only when the entire requested operation is finished successfully.
For multi-step processes, use false until all steps are complete.`), mcp.Required()),
			mcp.WithBoolean("need_user_attention", mcp.Description(`Whether the user needs to take action on the task.
Set to true if the task is in a failed state or if the user needs to take action to continue.`), mcp.Required()),
		),
		MakeHandler: handleCoderReportTask,
	},
	{
		Tool: mcp.NewTool("coder_whoami",
			mcp.WithDescription(`Get information about the currently logged-in Coder user.
Returns JSON with the user's profile including fields: id, username, email, created_at, status, roles, etc.
Use this to identify the current user context before performing workspace operations.
This tool is useful for verifying permissions and checking the user's identity.

Common errors:
- Authentication failure: The session may have expired
- Server unavailable: The Coder deployment may be unreachable`),
		),
		MakeHandler: handleCoderWhoami,
	},
	{
		Tool: mcp.NewTool("coder_list_templates",
			mcp.WithDescription(`List all templates available on the Coder deployment.
Returns JSON with detailed information about each template, including:
- Template name, ID, and description
- Creation/modification timestamps
- Version information
- Associated organization

Use this tool to discover available templates before creating workspaces.
Templates define the infrastructure and configuration for workspaces.

Common errors:
- Authentication failure: Check user permissions
- No templates available: The deployment may not have any templates configured`),
		),
		MakeHandler: handleCoderListTemplates,
	},
	{
		Tool: mcp.NewTool("coder_list_workspaces",
			mcp.WithDescription(`List workspaces available on the Coder deployment.
Returns JSON with workspace metadata including status, resources, and configurations.
Use this before other workspace operations to find valid workspace names/IDs.
Results are paginated - use offset and limit parameters for large deployments.

Common errors:
- Authentication failure: Check user permissions
- Invalid owner parameter: Ensure the owner exists`),
			mcp.WithString(`owner`, mcp.Description(`The username of the workspace owner to filter by.
Defaults to "me" which represents the currently authenticated user.
Use this to view workspaces belonging to other users (requires appropriate permissions).
Special value: "me" - List workspaces owned by the authenticated user.`), mcp.DefaultString(codersdk.Me)),
			mcp.WithNumber(`offset`, mcp.Description(`Pagination offset - the starting index for listing workspaces.
Used with the 'limit' parameter to implement pagination.
For example, to get the second page of results with 10 items per page, use offset=10.
Defaults to 0 (first page).`), mcp.DefaultNumber(0)),
			mcp.WithNumber(`limit`, mcp.Description(`Maximum number of workspaces to return in a single request.
Used with the 'offset' parameter to implement pagination.
Higher values return more results but may increase response time.
Valid range: 1-100. Defaults to 10.`), mcp.DefaultNumber(10)),
		),
		MakeHandler: handleCoderListWorkspaces,
	},
	{
		Tool: mcp.NewTool("coder_get_workspace",
			mcp.WithDescription(`Get detailed information about a specific Coder workspace.
Returns comprehensive JSON with the workspace's configuration, status, and resources.
Use this to check workspace status before performing operations like exec or start/stop.
The response includes the latest build status, agent connectivity, and resource details.

Common errors:
- Workspace not found: Check the workspace name or ID
- Permission denied: The user may not have access to this workspace`),
			mcp.WithString("workspace", mcp.Description(`The workspace ID (UUID) or name to retrieve.
Can be specified as either:
- Full UUID: e.g., "8a0b9c7d-1e2f-3a4b-5c6d-7e8f9a0b1c2d"
- Workspace name: e.g., "dev", "python-project"
Use coder_list_workspaces first if you're not sure about available workspace names.`), mcp.Required()),
		),
		MakeHandler: handleCoderGetWorkspace,
	},
	{
		Tool: mcp.NewTool("coder_workspace_exec",
			mcp.WithDescription(`Execute a shell command in a remote Coder workspace.
Runs the specified command and returns the complete output (stdout/stderr).
Use this for file operations, running build commands, or checking workspace state.
The workspace must be running with a connected agent for this to succeed.

Before using this tool:
1. Verify the workspace is running using coder_get_workspace
2. Start the workspace if needed using coder_start_workspace

Common errors:
- Workspace not running: Start the workspace first
- Command not allowed: Check security restrictions
- Agent not connected: The workspace may still be starting up`),
			mcp.WithString("workspace", mcp.Description(`The workspace ID (UUID) or name where the command will execute.
Can be specified as either:
- Full UUID: e.g., "8a0b9c7d-1e2f-3a4b-5c6d-7e8f9a0b1c2d"
- Workspace name: e.g., "dev", "python-project"
The workspace must be running with a connected agent.
Use coder_get_workspace first to check the workspace status.`), mcp.Required()),
			mcp.WithString("command", mcp.Description(`The shell command to execute in the workspace.
Commands are executed in the default shell of the workspace.

Examples:
- "ls -la" - List files with details
- "cd /path/to/directory && command" - Execute in specific directory
- "cat ~/.bashrc" - View a file's contents
- "python -m pip list" - List installed Python packages

Note: Very long-running commands may time out.`), mcp.Required()),
		),
		MakeHandler: handleCoderWorkspaceExec,
	},
	{
		Tool: mcp.NewTool("coder_workspace_transition",
			mcp.WithDescription(`Start or stop a running Coder workspace.
If stopping, initiates the workspace stop transition.
Only works on workspaces that are currently running or failed.

If starting, initiates the workspace start transition.
Only works on workspaces that are currently stopped or failed.

Stopping or starting a workspace is an asynchronous operation - it may take several minutes to complete.

After calling this tool:
1. Use coder_report_task to inform the user that the workspace is stopping or starting
2. Use coder_get_workspace periodically to check for completion

Common errors:
- Workspace already started/starting/stopped/stopping: No action needed
- Cancellation failed: There may be issues with the underlying infrastructure
- User doesn't own workspace: Permission issues`),
			mcp.WithString("workspace", mcp.Description(`The workspace ID (UUID) or name to start or stop.
Can be specified as either:
- Full UUID: e.g., "8a0b9c7d-1e2f-3a4b-5c6d-7e8f9a0b1c2d"
- Workspace name: e.g., "dev", "python-project"
The workspace must be in a running state to be stopped, or in a stopped or failed state to be started.
Use coder_get_workspace first to check the current workspace status.`), mcp.Required()),
			mcp.WithString("transition", mcp.Description(`The transition to apply to the workspace.
Can be either "start" or "stop".`)),
		),
		MakeHandler: handleCoderWorkspaceTransition,
	},
}

// ToolDeps contains all dependencies needed by tool handlers
type ToolDeps struct {
	Client        *codersdk.Client
	AgentClient   *agentsdk.Client
	Logger        *slog.Logger
	AppStatusSlug string
}

// ToolHandler associates a tool with its handler creation function
type ToolHandler struct {
	Tool        mcp.Tool
	MakeHandler func(ToolDeps) server.ToolHandlerFunc
}

// ToolRegistry is a map of available tools with their handler creation
// functions
type ToolRegistry []ToolHandler

// WithOnlyAllowed returns a new ToolRegistry containing only the tools
// specified in the allowed list.
func (r ToolRegistry) WithOnlyAllowed(allowed ...string) ToolRegistry {
	if len(allowed) == 0 {
		return []ToolHandler{}
	}

	filtered := make(ToolRegistry, 0, len(r))

	// The overhead of a map lookup is likely higher than a linear scan
	// for a small number of tools.
	for _, entry := range r {
		if slices.Contains(allowed, entry.Tool.Name) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// Register registers all tools in the registry with the given tool adder
// and dependencies.
func (r ToolRegistry) Register(srv *server.MCPServer, deps ToolDeps) {
	for _, entry := range r {
		srv.AddTool(entry.Tool, entry.MakeHandler(deps))
	}
}

// AllTools returns all available tools.
func AllTools() ToolRegistry {
	// return a copy of allTools to avoid mutating the original
	return slices.Clone(allTools)
}

type handleCoderReportTaskArgs struct {
	Summary           string `json:"summary"`
	Link              string `json:"link"`
	Emoji             string `json:"emoji"`
	Done              bool   `json:"done"`
	NeedUserAttention bool   `json:"need_user_attention"`
}

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_report_task", "arguments": {"summary": "I need help with the login page.", "link": "https://github.com/coder/coder/pull/1234", "emoji": "🔍", "done": false, "need_user_attention": true}}}
func handleCoderReportTask(deps ToolDeps) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if deps.AgentClient == nil {
			return nil, xerrors.New("developer error: agent client is required")
		}

		if deps.AppStatusSlug == "" {
			return nil, xerrors.New("No app status slug provided, set CODER_MCP_APP_STATUS_SLUG when running the MCP server to report tasks.")
		}

		// Convert the request parameters to a json.RawMessage so we can unmarshal
		// them into the correct struct.
		args, err := unmarshalArgs[handleCoderReportTaskArgs](request.Params.Arguments)
		if err != nil {
			return nil, xerrors.Errorf("failed to unmarshal arguments: %w", err)
		}

		deps.Logger.Info(ctx, "report task tool called",
			slog.F("summary", args.Summary),
			slog.F("link", args.Link),
			slog.F("emoji", args.Emoji),
			slog.F("done", args.Done),
			slog.F("need_user_attention", args.NeedUserAttention),
		)

		newStatus := agentsdk.PatchAppStatus{
			AppSlug:            deps.AppStatusSlug,
			Message:            args.Summary,
			URI:                args.Link,
			Icon:               args.Emoji,
			NeedsUserAttention: args.NeedUserAttention,
			State:              codersdk.WorkspaceAppStatusStateWorking,
		}

		if args.Done {
			newStatus.State = codersdk.WorkspaceAppStatusStateComplete
		}
		if args.NeedUserAttention {
			newStatus.State = codersdk.WorkspaceAppStatusStateFailure
		}

		if err := deps.AgentClient.PatchAppStatus(ctx, newStatus); err != nil {
			return nil, xerrors.Errorf("failed to patch app status: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent("Thanks for reporting!"),
			},
		}, nil
	}
}

// Example payload:
// {"jsonrpc":"2.0","id":1,"method":"tools/call", "params": {"name": "coder_whoami", "arguments": {}}}
func handleCoderWhoami(deps ToolDeps) server.ToolHandlerFunc {
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
func handleCoderListWorkspaces(deps ToolDeps) server.ToolHandlerFunc {
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
func handleCoderGetWorkspace(deps ToolDeps) server.ToolHandlerFunc {
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
func handleCoderWorkspaceExec(deps ToolDeps) server.ToolHandlerFunc {
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
func handleCoderListTemplates(deps ToolDeps) server.ToolHandlerFunc {
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
func handleCoderWorkspaceTransition(deps ToolDeps) server.ToolHandlerFunc {
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
