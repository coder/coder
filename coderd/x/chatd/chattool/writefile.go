package chattool

import (
	"context"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type WriteFileOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	ResolvePlanPath  func(context.Context) (chatPath string, home string, err error)
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
			return executeWriteFileTool(ctx, conn, args, options.ResolvePlanPath)
		},
	)
}

func executeWriteFileTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args WriteFileArgs,
	resolvePlanPath func(context.Context) (chatPath string, home string, err error),
) (fantasy.ToolResponse, error) {
	requestedPath := strings.TrimSpace(args.Path)
	if requestedPath == "" {
		return fantasy.NewTextErrorResponse("path is required"), nil
	}

	hasPlanFileName := looksLikePlanFileName(requestedPath)
	if hasPlanFileName && !isAbsolutePath(requestedPath) {
		return fantasy.NewTextErrorResponse(
			"plan files must use absolute paths; use the chat-specific absolute plan path",
		), nil
	}

	if resolvePlanPath != nil && hasPlanFileName {
		chatPath, home, err := resolvePlanPath(ctx)
		if resp, rejected := rejectSharedPlanPath(requestedPath, home, chatPath, err); rejected {
			return resp, nil
		}
	}

	if err := conn.WriteFile(ctx, requestedPath, strings.NewReader(args.Content)); err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	return toolResponse(map[string]any{"ok": true}), nil
}
