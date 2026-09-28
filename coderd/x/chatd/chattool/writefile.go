package chattool

import (
	"context"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

type WriteFileOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	ResolvePlanPath  func(context.Context) (chatPath string, home string, err error)
	IsPlanTurn       bool
	// Clock times the retries of requests the agent does not answer
	// (AgentAnswerTimeout). Nil means a real clock.
	Clock quartz.Clock
}

type WriteFileArgs struct {
	Path    string `json:"path" description:"Absolute path of the file to write. Plan files must use the chat-specific absolute plan path."`
	Content string `json:"content" description:"Complete file contents. Replaces any existing contents."`
}

func WriteFile(options WriteFileOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		WriteFileToolName,
		"Create a file in the workspace or overwrite an existing one with the given content. "+
			"Use edit_files for targeted changes to an existing file. "+
			"During plan turns, only the chat-specific plan file path is writable.",
		func(ctx context.Context, args WriteFileArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			id, hasID := ToolCallIdentityFromContext(ctx)
			// The interrupt run skips the checks that depend on the
			// chat's state, which may have changed since the request was
			// first sent: the agent answers a replay with the recorded
			// response and refuses a first request after the cancel.
			interrupt := hasID && id.Cause == ToolCallCauseInterrupt
			var planPath string
			if options.IsPlanTurn && !interrupt {
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
				if hasID {
					if text, ok := fileConnErrorText(err, writeFileWords(id)); ok {
						return fantasy.NewTextErrorResponse(text), nil
					}
				}
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if planPath != "" {
				if err := ensurePlanPathResolvesToItself(ctx, conn, planPath); err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
			}
			resolvePlanPath := options.ResolvePlanPath
			if interrupt {
				// After the cancel the agent never applies the write, so
				// the request below replays the recorded response or gets
				// tool_call_canceled.
				if err := cancelToolCall(ctx, conn, options.Clock, id); err != nil {
					text, _ := cancelErrorText(err, writeFileWords(id))
					return fantasy.NewTextErrorResponse(text), nil
				}
				resolvePlanPath = nil
			}
			return executeWriteFileTool(ctx, conn, options.Clock, args, resolvePlanPath)
		},
	)
}

func executeWriteFileTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	clock quartz.Clock,
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

	var err error
	if id, ok := ToolCallIdentityFromContext(ctx); ok {
		err = RequestUntilAnswered(workspacesdk.WithToolCall(ctx, id.AgentToolCall()), clock, func(ctx context.Context) error {
			// Every send streams the content from the start.
			return conn.WriteFile(ctx, requestedPath, strings.NewReader(args.Content))
		})
		if text, ok := ToolCallErrorText(err, writeFileWords(id)); ok {
			return fantasy.NewTextErrorResponse(text), nil
		}
	} else {
		err = conn.WriteFile(ctx, requestedPath, strings.NewReader(args.Content))
	}
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	return toolResponse(map[string]any{"ok": true}), nil
}

// writeFileWords are the words write_file uses in tool call results.
func writeFileWords(id ToolCallIdentity) ToolCallWords {
	return fileToolWords("write", id)
}
