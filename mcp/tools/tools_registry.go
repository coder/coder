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
			mcp.WithDescription(`Report progress on a task.`),
			mcp.WithString("summary", mcp.Description(`A summary of your progress on a task.
	
Good Summaries:
- "Taking a look at the login page..."
- "Found a bug! Fixing it now..."
- "Investigating the GitHub Issue..."`), mcp.Required()),
			mcp.WithString("link", mcp.Description(`A relevant link to your work. e.g. GitHub issue link, pull request link, etc.`), mcp.Required()),
			mcp.WithString("emoji", mcp.Description(`A relevant emoji to your work.`), mcp.Required()),
			mcp.WithBoolean("done", mcp.Description(`Whether the task the user requested is complete.`), mcp.Required()),
		),
		MakeHandler: handleCoderReportTask,
	},
	{
		Tool: mcp.NewTool("coder_whoami",
			mcp.WithDescription(`Get information about the currently logged-in Coder user.`),
		),
		MakeHandler: handleCoderWhoami,
	},
	{
		Tool: mcp.NewTool("coder_list_templates",
			mcp.WithDescription(`List all templates on a given Coder deployment.`),
		),
		MakeHandler: handleCoderListTemplates,
	},
	{
		Tool: mcp.NewTool("coder_list_workspaces",
			mcp.WithDescription(`List workspaces on a given Coder deployment owned by the current user.`),
			mcp.WithString(`owner`, mcp.Description(`The owner of the workspaces to list. Defaults to the current user.`), mcp.DefaultString(codersdk.Me)),
			mcp.WithNumber(`offset`, mcp.Description(`The offset to start listing workspaces from. Defaults to 0.`), mcp.DefaultNumber(0)),
			mcp.WithNumber(`limit`, mcp.Description(`The maximum number of workspaces to list. Defaults to 10.`), mcp.DefaultNumber(10)),
		),
		MakeHandler: handleCoderListWorkspaces,
	},
	{
		Tool: mcp.NewTool("coder_get_workspace",
			mcp.WithDescription(`Get information about a workspace on a given Coder deployment.`),
			mcp.WithString("workspace", mcp.Description(`The workspace ID or name to get.`), mcp.Required()),
		),
		MakeHandler: handleCoderGetWorkspace,
	},
	{
		Tool: mcp.NewTool("coder_workspace_exec",
			mcp.WithDescription(`Execute a command in a remote workspace on a given Coder deployment.`),
			mcp.WithString("workspace", mcp.Description(`The workspace ID or name in which to execute the command in. The workspace must be running.`), mcp.Required()),
			mcp.WithString("command", mcp.Description(`The command to execute. Changing the working directory is not currently supported, so you may need to preface the command with 'cd /some/path && <my-command>'.`), mcp.Required()),
		),
		MakeHandler: handleCoderWorkspaceExec,
	},
	{
		Tool: mcp.NewTool("coder_start_workspace",
			mcp.WithDescription(`Start a workspace on a given Coder deployment.`),
			mcp.WithString("workspace", mcp.Description(`The workspace ID or name to start.`), mcp.Required()),
		),
		MakeHandler: handleCoderStartWorkspace,
	},
	{
		Tool: mcp.NewTool("coder_stop_workspace",
			mcp.WithDescription(`Stop a workspace on a given Coder deployment.`),
			mcp.WithString("workspace", mcp.Description(`The workspace ID or name to stop.`), mcp.Required()),
		),
		MakeHandler: handleCoderStopWorkspace,
	},
}

// ToolAdder interface for adding tools to a server
type ToolAdder interface {
	AddTool(tool mcp.Tool, handler server.ToolHandlerFunc)
}

// Ensure that MCPServer implements ToolAdder
var _ ToolAdder = (*server.MCPServer)(nil)

// ToolDeps contains all dependencies needed by tool handlers
type ToolDeps struct {
	Client              *codersdk.Client
	Logger              *slog.Logger
	AllowedExecCommands []string
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
func (r ToolRegistry) Register(ta ToolAdder, deps ToolDeps) {
	for _, entry := range r {
		ta.AddTool(entry.Tool, entry.MakeHandler(deps))
	}
}

// AllTools returns all available tools.
func AllTools() ToolRegistry {
	// return a copy of allTools to avoid mutating the original
	return slices.Clone(allTools)
}
