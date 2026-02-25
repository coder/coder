package chattool

import (
	"context"
	"io"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
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
		func(ctx context.Context, args ReadFileArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			result := chatprompt.ToolResultBlock{
				ToolCallID: call.ID,
				ToolName:   call.Name,
			}
			if options.GetWorkspaceConn == nil {
				return toolResultBlockToAgentResponse(
					toolError(result, xerrors.New("workspace connection resolver is not configured")),
				), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return toolResultBlockToAgentResponse(toolError(result, err)), nil
			}
			return toolResultBlockToAgentResponse(executeReadFileTool(ctx, conn, result, args)), nil
		},
	)
}

func executeReadFileTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	result chatprompt.ToolResultBlock,
	args ReadFileArgs,
) chatprompt.ToolResultBlock {
	if args.Path == "" {
		return toolError(result, xerrors.New("path is required"))
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
		return toolError(result, err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return toolError(result, err)
	}

	result.Result = map[string]any{
		"content":   string(data),
		"mime_type": mimeType,
	}
	return result
}
