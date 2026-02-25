package chattool

import (
	"context"
	"strings"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
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
		func(ctx context.Context, args WriteFileArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
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
			return toolResultBlockToAgentResponse(executeWriteFileTool(ctx, conn, result, args)), nil
		},
	)
}

func executeWriteFileTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	result chatprompt.ToolResultBlock,
	args WriteFileArgs,
) chatprompt.ToolResultBlock {
	if args.Path == "" {
		return toolError(result, xerrors.New("path is required"))
	}

	if err := conn.WriteFile(ctx, args.Path, strings.NewReader(args.Content)); err != nil {
		return toolError(result, err)
	}
	result.Result = map[string]any{"ok": true}
	return result
}
