package toolsdk

import (
	"context"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
)

// ToolNameExportWorkspaceExecution identifies explicit result preservation.
const ToolNameExportWorkspaceExecution = "coder_export_workspace_execution"

// ExportWorkspaceExecution preserves declared results without deleting the workspace.
var ExportWorkspaceExecution = Tool[WorkspaceExecutionControlArgs, []codersdk.WorkspaceExecutionArtifact]{
	Tool: aisdk.Tool{
		Name:        ToolNameExportWorkspaceExecution,
		Description: "Preserve this session's declared result files as one immutable durable generation, including retained or reused workspaces. Tracked workspace commands and chat work must settle first. For a reused workspace, stop preexisting untracked commands and writers before exporting; their completion cannot be observed by this session. Closes workspace admission while collecting or recovering failed preservation. A successful adopted-workspace export ends this session and permits subsequent work through another session; a disposable workspace stays closed. Does not change retention or schedule deletion. Retry the same expected_revision to recover after a lost response. Existing sessions can still export their declared results.",
		Schema:      aisdk.Schema{Properties: executionControlProperties(), Required: []string{"organization_id", "session_id", "expected_revision"}},
	},
	MCPAnnotations: mcpMutationAnnotations,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceExecutionControlArgs) ([]codersdk.WorkspaceExecutionArtifact, error) {
		org, err := resolveOrganization(ctx, deps, args.OrganizationID)
		if err != nil {
			return nil, err
		}
		return deps.coderClient.ExportWorkspaceExecution(ctx, org, args.SessionID, codersdk.ExportWorkspaceExecutionRequest{ExpectedRevision: args.ExpectedRevision})
	},
}
