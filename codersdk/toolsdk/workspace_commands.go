package toolsdk

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
)

const (
	ToolNameStartWorkspaceCommand  = "coder_start_workspace_command"
	ToolNameGetWorkspaceCommand    = "coder_get_workspace_command"
	ToolNameCancelWorkspaceCommand = "coder_cancel_workspace_command"
)

// StartWorkspaceCommandArgs selects a session and stable command intent.
type StartWorkspaceCommandArgs struct {
	OrganizationID string    `json:"organization_id"`
	SessionID      uuid.UUID `json:"session_id"`
	codersdk.StartWorkspaceCommandRequest
}

// WorkspaceCommandArgs selects an existing receipt and bounded observation wait.
type WorkspaceCommandArgs struct {
	OrganizationID string    `json:"organization_id"`
	SessionID      uuid.UUID `json:"session_id"`
	ExecutionID    uuid.UUID `json:"execution_id"`
	WaitMillis     int64     `json:"wait_ms,omitempty"`
}

// StartWorkspaceCommand records a command before dispatch and recovers its receipt on retry.
var StartWorkspaceCommand = Tool[StartWorkspaceCommandArgs, codersdk.WorkspaceCommand]{
	Tool: aisdk.Tool{
		Name: ToolNameStartWorkspaceCommand, Description: "Start an asynchronous command in an acquired execution session. Preserve request_id and exact command input for retries: identical requests recover the original execution, never run again. Unsupported agents reject admission. Unknown means dispatch or outcome is uncertain, never permission to resubmit with a new identity. Session lease and declared execution deadline apply; this call does not wait for completion.",
		Schema: aisdk.Schema{Properties: map[string]any{
			"organization_id": map[string]any{"type": "string"},
			"session_id":      map[string]any{"type": "string", "format": "uuid"},
			"request_id":      map[string]any{"type": "string", "format": "uuid"},
			"agent_id":        map[string]any{"type": "string", "format": "uuid"},
			"command":         map[string]any{"type": "string"},
			"workdir":         map[string]any{"type": "string"},
			"env":             map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
		}, Required: []string{"organization_id", "session_id", "request_id", "agent_id", "command"}},
	},
	MCPAnnotations: MCPToolAnnotations{IdempotentHint: true, DestructiveHint: true, OpenWorldHint: true},
	Handler: func(ctx context.Context, deps Deps, args StartWorkspaceCommandArgs) (codersdk.WorkspaceCommand, error) {
		org, err := resolveOrganization(ctx, deps, args.OrganizationID)
		if err != nil {
			return codersdk.WorkspaceCommand{}, err
		}
		return deps.coderClient.StartWorkspaceCommand(ctx, org, args.SessionID, args.StartWorkspaceCommandRequest)
	},
}

func workspaceCommandSchema() aisdk.Schema {
	return aisdk.Schema{Properties: map[string]any{
		"organization_id": map[string]any{"type": "string"},
		"session_id":      map[string]any{"type": "string", "format": "uuid"},
		"execution_id":    map[string]any{"type": "string", "format": "uuid"},
		"wait_ms":         map[string]any{"type": "integer", "minimum": 0, "maximum": 30000},
	}, Required: []string{"organization_id", "session_id", "execution_id"}}
}

// GetWorkspaceCommand reads bounded output and actual observed execution state.
var GetWorkspaceCommand = Tool[WorkspaceCommandArgs, codersdk.WorkspaceCommand]{
	Tool:           aisdk.Tool{Name: ToolNameGetWorkspaceCommand, Description: "Read command status and bounded output without starting work or renewing a lease. wait_ms bounds observation only; returning running with no exit code does not cancel execution. Output is ephemeral and may be unavailable after agent restart or history expiry. Durable known exit remains available; preserve required files as declared artifacts. Truncation reports original, retained including markers, and omitted source bytes.", Schema: workspaceCommandSchema()},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, args WorkspaceCommandArgs) (codersdk.WorkspaceCommand, error) {
		org, err := resolveOrganization(ctx, deps, args.OrganizationID)
		if err != nil {
			return codersdk.WorkspaceCommand{}, err
		}
		return deps.coderClient.WorkspaceCommand(ctx, org, args.SessionID, args.ExecutionID, args.WaitMillis)
	},
}

// CancelWorkspaceCommand cancels the original command without inferring an exit.
var CancelWorkspaceCommand = Tool[WorkspaceCommandArgs, codersdk.WorkspaceCommand]{
	Tool:           aisdk.Tool{Name: ToolNameCancelWorkspaceCommand, Description: "Cancel the original command, or fence a planned start that has not arrived. Completed requires observed terminal acknowledgment; running or unknown means cancellation has not been confirmed. A bounded wait is independent from the execution deadline. Safe to repeat with the same execution_id.", Schema: workspaceCommandSchema()},
	MCPAnnotations: MCPToolAnnotations{IdempotentHint: true, DestructiveHint: true},
	Handler: func(ctx context.Context, deps Deps, args WorkspaceCommandArgs) (codersdk.WorkspaceCommand, error) {
		org, err := resolveOrganization(ctx, deps, args.OrganizationID)
		if err != nil {
			return codersdk.WorkspaceCommand{}, err
		}
		return deps.coderClient.CancelWorkspaceCommand(ctx, org, args.SessionID, args.ExecutionID, args.WaitMillis)
	},
}
