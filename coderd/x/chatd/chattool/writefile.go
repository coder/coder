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
	IsPlanTurn       bool
}

type WriteFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func WriteFile(options WriteFileOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		WriteFileToolName,
		"Write a file to the workspace.",
		func(ctx context.Context, args WriteFileArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			var planPath string
			if options.IsPlanTurn {
				args.Path = strings.TrimSpace(args.Path)
				resolvedPlanPath, err := resolvePlanTurnPath(ctx, options.ResolvePlanPath)
				if err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
				if args.Path != resolvedPlanPath {
					return fantasy.NewTextErrorResponse("during plan turns, write_file is restricted to " + resolvedPlanPath), nil
				}
				planPath = resolvedPlanPath
			}
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fileToolConnErrorResult(ctx, WriteFileToolName, err), nil
			}
			if planPath != "" {
				if err := ensurePlanPathResolvesToItself(ctx, conn, planPath); err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
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

	writeCtx := ctx
	if id, ok := ToolCallIdentityFromContext(ctx); ok {
		writeCtx = workspacesdk.WithToolCall(ctx, id.AgentToolCall())
	}
	err := conn.WriteFile(writeCtx, requestedPath, strings.NewReader(args.Content))
	if result, ok := fileRequestErrorResult(ctx, WriteFileToolName, err); ok {
		return result, nil
	}
	return writeFileResult(err), nil
}

// writeFileResult converts an answer the agent gave to a write_file
// request, live or recorded, into the tool result.
func writeFileResult(err error) fantasy.ToolResponse {
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error())
	}
	return toolResponse(map[string]any{"ok": true})
}
