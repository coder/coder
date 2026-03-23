package chattool

import (
	"context"
	"io"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const maxProposePlanSize = 32 * 1024 // 32 KiB

type ProposePlanOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
}

type ProposePlanArgs struct {
	Path string `json:"path"`
}

func ProposePlan(options ProposePlanOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"propose_plan",
		"Present a Markdown plan file from the workspace for user review. "+
			"The file must already exist — use write_file to create it or edit_files to refine it before calling this tool. "+
			"Pass the absolute file path (e.g. /home/coder/PLAN.md). The tool reads the content from the workspace.",
		func(ctx context.Context, args ProposePlanArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return executeProposePlanTool(ctx, conn, args)
		},
	)
}

func executeProposePlanTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args ProposePlanArgs,
) (fantasy.ToolResponse, error) {
	path := strings.TrimSpace(args.Path)
	if path == "" {
		return fantasy.NewTextErrorResponse("path is required (use an absolute path, e.g. /home/coder/PLAN.md)"), nil
	}
	if !strings.HasSuffix(path, ".md") {
		return fantasy.NewTextErrorResponse("path must end with .md"), nil
	}

	rc, _, err := conn.ReadFile(ctx, path, 0, maxProposePlanSize+1)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	if int64(len(data)) > maxProposePlanSize {
		return fantasy.NewTextErrorResponse("plan file exceeds 32 KiB size limit"), nil
	}

	return toolResponse(map[string]any{"ok": true, "path": path, "kind": "plan", "content": string(data)}), nil
}
