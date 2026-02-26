package chattool

import (
	"context"
	"io"

	"charm.land/fantasy"

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
		"Read a file from the workspace.",
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
	if args.Path == "" {
		return fantasy.NewTextErrorResponse("path is required"), nil
	}

	offset := int64(0)
	limit := int64(0)
	if args.Offset != nil {
		offset = *args.Offset
	}
	if args.Limit != nil {
		limit = *args.Limit
	}

	reader, mimeType, err := conn.ReadFile(ctx, args.Path, offset, limit)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	return toolResponse(map[string]any{
		"content":   string(data),
		"mime_type": mimeType,
	}), nil
}
