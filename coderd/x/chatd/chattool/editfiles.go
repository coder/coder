package chattool

import (
	"context"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type EditFilesOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	ResolvePlanPath  func(context.Context) (chatPath string, home string, err error)
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
			return executeEditFilesTool(ctx, conn, args, options.ResolvePlanPath)
		},
	)
}

func executeEditFilesTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args EditFilesArgs,
	resolvePlanPath func(context.Context) (chatPath string, home string, err error),
) (fantasy.ToolResponse, error) {
	if len(args.Files) == 0 {
		return fantasy.NewTextErrorResponse("files is required"), nil
	}

	var (
		chatPath       string
		home           string
		planPathErr    error
		planPathLoaded bool
	)
	for i := range args.Files {
		args.Files[i].Path = strings.TrimSpace(args.Files[i].Path)
		file := args.Files[i]

		hasPlanFileName := looksLikePlanFileName(file.Path)
		if hasPlanFileName && !isAbsolutePath(file.Path) {
			return fantasy.NewTextErrorResponse(
				"plan files must use absolute paths; use the chat-specific absolute plan path; no files in this batch were applied",
			), nil
		}
		if resolvePlanPath == nil || !hasPlanFileName {
			continue
		}
		if !planPathLoaded {
			chatPath, home, planPathErr = resolvePlanPath(ctx)
			planPathLoaded = true
		}
		if resp, rejected := rejectSharedPlanPath(file.Path, home, chatPath, planPathErr); rejected {
			return fantasy.NewTextErrorResponse(
				resp.Content + "; no files in this batch were applied",
			), nil
		}
	}

	if err := conn.EditFiles(ctx, workspacesdk.FileEditRequest{Files: args.Files}); err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	return toolResponse(map[string]any{"ok": true}), nil
}
