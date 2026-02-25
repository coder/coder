package chattool

import (
	"context"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type EditFilesOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
}

type EditFilesArgs struct {
	Files []workspacesdk.FileEdits `json:"files"`
}

func EditFiles(options EditFilesOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"edit_files",
		"Perform search-and-replace edits on one or more files in the workspace."+
			" Each file can have multiple edits applied atomically.",
		func(ctx context.Context, args EditFilesArgs, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
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
			return toolResultBlockToAgentResponse(executeEditFilesTool(ctx, conn, result, args)), nil
		},
	)
}

func executeEditFilesTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	result chatprompt.ToolResultBlock,
	args EditFilesArgs,
) chatprompt.ToolResultBlock {
	if len(args.Files) == 0 {
		return toolError(result, xerrors.New("files is required"))
	}

	if err := conn.EditFiles(ctx, workspacesdk.FileEditRequest{Files: args.Files}); err != nil {
		return toolError(result, err)
	}
	result.Result = map[string]any{"ok": true}
	return result
}
