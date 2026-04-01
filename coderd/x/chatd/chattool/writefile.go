package chattool

import (
	"context"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type WriteFileOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
}

type WriteFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func WriteFile(options WriteFileOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"write_file",
		"Write a file to the workspace.",
		func(ctx context.Context, args WriteFileArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return executeWriteFileTool(ctx, conn, args)
		},
	)
}

func executeWriteFileTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args WriteFileArgs,
) (fantasy.ToolResponse, error) {
	if args.Path == "" {
		return fantasy.NewTextErrorResponse("path is required"), nil
	}

	if err := conn.WriteFile(ctx, args.Path, strings.NewReader(args.Content)); err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	return toolResponse(map[string]any{"ok": true}), nil
}
