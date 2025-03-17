package agentmcp

import (
	"context"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func New(ctx context.Context) error {
	srv := server.NewMCPServer(
		"Coder Agent",
		buildinfo.Version(),
	)

	tool := mcp.NewTool("summarize_task",
		mcp.WithDescription("Summarize your progress on a task."))

	srv.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.NewTextContent("Task summarized."),
			},
		}, nil
	})

	return server.ServeStdio(srv)
}