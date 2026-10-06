package chattool

import (
	"context"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/toolsdk/workspacetools"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type ReadFileOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
}

type ReadFileArgs struct {
	Path   string `json:"path"`
	Offset *int64 `json:"offset,omitempty"`
	Limit  *int64 `json:"limit,omitempty"`
}

func ReadFile(options ReadFileOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"read_file",
		workspacetools.ReadFileDescription,
		func(ctx context.Context, args ReadFileArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return executeReadFileTool(ctx, conn, args)
		},
	)
}

func executeReadFileTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args ReadFileArgs,
) (fantasy.ToolResponse, error) {
	result, err := workspacetools.ReadFile(ctx, conn, args.Path, args.Offset, args.Limit)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	return marshalToolResponse(result), nil
}
