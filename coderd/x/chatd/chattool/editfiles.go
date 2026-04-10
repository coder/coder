package chattool

import (
	"context"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type EditFilesOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	PlanPath         func(context.Context) (string, error)
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
			return executeEditFilesTool(ctx, conn, args, options.PlanPath)
		},
	)
}

func executeEditFilesTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args EditFilesArgs,
	resolvePlanPath func(context.Context) (string, error),
) (fantasy.ToolResponse, error) {
	if len(args.Files) == 0 {
		return fantasy.NewTextErrorResponse("files is required"), nil
	}

	for _, file := range args.Files {
		if resp, rejected := rejectSharedPlanPath(ctx, file.Path, resolvePlanPath); rejected {
			return resp, nil
		}
	}

	if err := conn.EditFiles(ctx, workspacesdk.FileEditRequest{Files: args.Files}); err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	return toolResponse(map[string]any{"ok": true}), nil
}
