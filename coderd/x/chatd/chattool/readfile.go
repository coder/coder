package chattool

import (
	"context"

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
		"Read a file from the workspace. Returns line-numbered content. "+
			"The offset parameter is a 1-based line number (default: 1). "+
			"The limit parameter is the number of lines to return (default: 2000). "+
			"For large files, use offset and limit to paginate.",
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

	offset := int64(1) // 1-based line number default
	limit := int64(0)  // 0 means use server default (2000)
	if args.Offset != nil {
		offset = *args.Offset
	}
	if args.Limit != nil {
		limit = *args.Limit
	}

	resp, err := conn.ReadFileLines(ctx, args.Path, offset, limit, workspacesdk.DefaultReadFileLinesLimits())
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	if !resp.Success {
		return fantasy.NewTextErrorResponse(resp.Error), nil
	}

	return toolResponse(map[string]any{
		"content":     resp.Content,
		"file_size":   resp.FileSize,
		"total_lines": resp.TotalLines,
		"lines_read":  resp.LinesRead,
	}), nil
}
