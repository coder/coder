package toolsdk

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
)

const (
	ToolNameAcquireWorkspaceExecution    = "coder_acquire_workspace_execution"
	ToolNameGetWorkspaceExecutionSession = "coder_get_workspace_execution_session"
)

// AcquireWorkspaceExecutionArgs identifies an organization and immutable acquisition input.
type AcquireWorkspaceExecutionArgs struct {
	OrganizationID string `json:"organization_id"`
	codersdk.AcquireWorkspaceExecutionRequest
}

// AcquireWorkspaceExecution acquires or recovers an actor-scoped workspace session.
var AcquireWorkspaceExecution = Tool[AcquireWorkspaceExecutionArgs, codersdk.WorkspaceExecutionSession]{
	Tool: aisdk.Tool{
		Name:        ToolNameAcquireWorkspaceExecution,
		Description: "Acquire a workspace execution session using a stable request_id. Repeating identical input recovers the original workspace and build, including after a lost response. Supply exactly one of create or workspace_id, explicit retained choice, and an explicit lease expiration. Disposable authority is available only for newly created workspaces. Acceptance does not imply provisioning or agent readiness; inspect acquisition_build and readiness. Quota is advisory until the provisioner plan runs.",
		Schema: aisdk.Schema{
			Properties: map[string]any{
				"organization_id": map[string]any{"type": "string", "description": "Organization UUID."},
				"request_id":      map[string]any{"type": "string", "format": "uuid", "description": "Stable UUID for this immutable acquisition intent; preserve across retries."},
				"owner_id":        map[string]any{"type": "string", "format": "uuid"},
				"workspace_id":    map[string]any{"type": "string", "format": "uuid"},
				"create": map[string]any{"type": "object", "description": "New workspace input. Specify exactly one template ID or template version ID.", "required": []string{"name"}, "additionalProperties": false, "properties": map[string]any{
					"name":                       map[string]any{"type": "string"},
					"template_id":                map[string]any{"type": "string", "format": "uuid"},
					"template_version_id":        map[string]any{"type": "string", "format": "uuid"},
					"template_version_preset_id": map[string]any{"type": "string", "format": "uuid"},
					"autostart_schedule":         map[string]any{"type": "string"},
					"ttl_ms":                     map[string]any{"type": "integer"},
					"automatic_updates":          map[string]any{"type": "string"},
					"rich_parameter_values":      map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"name", "value"}, "properties": map[string]any{"name": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"}}, "additionalProperties": false}},
				}},
				"lease_expires_at": map[string]any{"type": "string", "format": "date-time"},
				"disposable":       map[string]any{"type": "boolean"},
				"retained":         map[string]any{"type": "boolean", "description": "Explicit retention choice. A retained workspace is protected from disposal."},
				"declarations": map[string]any{"type": "object", "properties": map[string]any{
					"result_agent_name":   map[string]any{"type": "string", "description": "Immutable unique agent name known from the template before creation. Mutually exclusive with result_agent_id; missing or ambiguous names fail preservation."},
					"result_agent_id":     map[string]any{"type": "string", "format": "uuid", "description": "Optional immutable result collection agent. Without one, preservation requires exactly one agent; ambiguity fails rather than choosing arbitrarily."},
					"result_paths":        map[string]any{"type": []string{"array", "null"}, "items": map[string]any{"type": "string"}},
					"execution_deadline":  map[string]any{"type": "string", "format": "date-time"},
					"artifact_expires_at": map[string]any{"type": "string", "format": "date-time"},
				}, "additionalProperties": false},
			},
			Required: []string{"organization_id", "request_id", "owner_id", "lease_expires_at", "retained"},
		},
	},
	MCPAnnotations: MCPToolAnnotations{ReadOnlyHint: false, DestructiveHint: false, IdempotentHint: true, OpenWorldHint: false},
	Handler: func(ctx context.Context, deps Deps, args AcquireWorkspaceExecutionArgs) (codersdk.WorkspaceExecutionSession, error) {
		org, err := resolveOrganization(ctx, deps, args.OrganizationID)
		if err != nil {
			return codersdk.WorkspaceExecutionSession{}, err
		}
		return deps.coderClient.AcquireWorkspaceExecution(ctx, org, args.AcquireWorkspaceExecutionRequest)
	},
}

// GetWorkspaceExecutionSessionArgs identifies an organization-scoped session.
type GetWorkspaceExecutionSessionArgs struct {
	OrganizationID string    `json:"organization_id"`
	SessionID      uuid.UUID `json:"session_id"`
}

// GetWorkspaceExecutionSession reads a durable receipt and its original build status.
var GetWorkspaceExecutionSession = Tool[GetWorkspaceExecutionSessionArgs, codersdk.WorkspaceExecutionSession]{
	Tool: aisdk.Tool{
		Name:        ToolNameGetWorkspaceExecutionSession,
		Description: "Read an organization-scoped execution session and the exact acquisition build status, including provisioning error and error code. Does not start or mutate the workspace. Stable IDs remain available if the source is removed.",
		Schema: aisdk.Schema{Properties: map[string]any{
			"organization_id": map[string]any{"type": "string"},
			"session_id":      map[string]any{"type": "string", "format": "uuid"},
		}, Required: []string{"organization_id", "session_id"}},
	},
	MCPAnnotations: mcpReadOnlyAnnotations,
	Handler: func(ctx context.Context, deps Deps, args GetWorkspaceExecutionSessionArgs) (codersdk.WorkspaceExecutionSession, error) {
		org, err := resolveOrganization(ctx, deps, args.OrganizationID)
		if err != nil {
			return codersdk.WorkspaceExecutionSession{}, err
		}
		return deps.coderClient.WorkspaceExecutionSession(ctx, org, args.SessionID)
	},
}
