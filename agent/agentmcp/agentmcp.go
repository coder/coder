package agentmcp

import (
	"context"
	"errors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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

	tool := mcp.NewTool("report_task",
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

	srv.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	return server.ServeStdio(srv)
}
