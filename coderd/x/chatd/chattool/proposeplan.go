package chattool

import (
	"context"
	"io"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const maxProposePlanSize = 32 * 1024 // 32 KiB

// ProposePlanOptions configures the propose_plan tool.
type ProposePlanOptions struct {
	GetWorkspaceConn func(context.Context) (workspacesdk.AgentConn, error)
	ResolvePlanPath  func(context.Context) (chatPath string, home string, err error)
	StoreFile        func(ctx context.Context, name string, mediaType string, data []byte) (uuid.UUID, error)
}

// ProposePlanArgs are the arguments for the propose_plan tool.
type ProposePlanArgs struct {
	Path string `json:"path"`
}

// ProposePlan returns a tool that presents a Markdown plan file from the
// workspace for user review.
func ProposePlan(options ProposePlanOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"propose_plan",
		"Present a Markdown plan file from the workspace for user review. "+
			"The file must already exist with a .md extension. Use write_file to create it or edit_files to refine it before calling this tool. "+
			"Pass the absolute file path to the plan. Important: use the chat-specific absolute plan path, not a generic path like PLAN.md in the home directory. "+
			"The tool reads the content from the workspace.",
		func(ctx context.Context, args ProposePlanArgs, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			if options.StoreFile == nil {
				return fantasy.NewTextErrorResponse("file storage is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return executeProposePlanTool(ctx, conn, args, options.ResolvePlanPath, options.StoreFile)
		},
	)
}

func executeProposePlanTool(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	args ProposePlanArgs,
	resolvePlanPath func(context.Context) (chatPath string, home string, err error),
	storeFile func(ctx context.Context, name string, mediaType string, data []byte) (uuid.UUID, error),
) (fantasy.ToolResponse, error) {
	requestedPath := strings.TrimSpace(args.Path)
	if requestedPath == "" {
		return fantasy.NewTextErrorResponse("path is required (use the chat-specific absolute plan path)"), nil
	}
	if !strings.HasSuffix(requestedPath, ".md") {
		return fantasy.NewTextErrorResponse("path must end with .md"), nil
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

	rc, _, err := conn.ReadFile(ctx, requestedPath, 0, maxProposePlanSize+1)
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

	fileID, err := storeFile(ctx, filepath.Base(requestedPath), "text/markdown", data)
	if err != nil {
		return fantasy.NewTextErrorResponse("failed to store plan file: " + err.Error()), nil
	}

	return toolResponse(map[string]any{
		"ok":         true,
		"path":       requestedPath,
		"kind":       "plan",
		"file_id":    fileID.String(),
		"media_type": "text/markdown",
	}), nil
}
