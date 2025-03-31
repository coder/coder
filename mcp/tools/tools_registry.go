package mcptools

import (
	"slices"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/codersdk"
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
	Client *codersdk.Client
	Logger *slog.Logger
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
