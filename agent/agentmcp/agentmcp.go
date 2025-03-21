package agentmcp

import (
	"context"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/agent/agentmcp/mcptools"
	"github.com/coder/coder/v2/buildinfo"
)

func New(ctx context.Context) error {
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

	logger := slog.Make(sloghuman.Sink(os.Stdout))

	mcptools.RegisterCoderReportTask(srv, logger)
	mcptools.RegisterCoderWhoami(srv)
	mcptools.RegisterCoderListWorkspaces(srv)
	mcptools.RegisterCoderWorkspaceExec(srv)

	return server.ServeStdio(srv)
}
