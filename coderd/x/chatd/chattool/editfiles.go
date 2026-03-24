package chattool

import (
	"context"
	"encoding/json"

	"charm.land/fantasy"

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
		func(ctx context.Context, args EditFilesArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return executeEditFilesTool(ctx, conn, args)
		},
	)
}

func executeEditFilesTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args EditFilesArgs,
) (fantasy.ToolResponse, error) {
	if len(args.Files) == 0 {
		return fantasy.NewTextErrorResponse("files is required"), nil
	}

	resp, err := conn.EditFiles(ctx, workspacesdk.FileEditRequest{Files: args.Files})
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return fantasy.NewTextErrorResponse("failed to marshal response: " + err.Error()), nil
	}
	return fantasy.NewTextResponse(string(data)), nil
}
