package toolsdk

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/coder/aisdk-go"
	"github.com/coder/coder/v2/codersdk"
)

const (
	ToolNameRenewWorkspaceExecutionSession  = "coder_renew_workspace_execution_session"
	ToolNameRetainWorkspaceExecutionSession = "coder_retain_workspace_execution_session"
	ToolNameRetryWorkspaceExecutionSession  = "coder_retry_workspace_execution_session"
)

// WorkspaceExecutionControlArgs targets one observed session revision.
type WorkspaceExecutionControlArgs struct {
	OrganizationID    string     `json:"organization_id"`
	SessionID         uuid.UUID  `json:"session_id"`
	ExpectedRevision  int64      `json:"expected_revision"`
	ArtifactExpiresAt *time.Time `json:"artifact_expires_at,omitempty"`
}

// RenewWorkspaceExecutionArgs supplies an explicit extension to the lease.
type RenewWorkspaceExecutionArgs struct {
	WorkspaceExecutionControlArgs
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
}

func executionControlProperties() map[string]any {
	return map[string]any{
		"organization_id":   map[string]any{"type": "string", "format": "uuid"},
		"session_id":        map[string]any{"type": "string", "format": "uuid"},
		"expected_revision": map[string]any{"type": "integer", "minimum": 1, "description": "Revision from the most recently observed session. A conflict requires refreshing state before another mutation."},
	}
}

// RenewWorkspaceExecutionSession extends a lease without releasing retention.
var RenewWorkspaceExecutionSession = Tool[RenewWorkspaceExecutionArgs, codersdk.WorkspaceExecutionControlReceipt]{
	Tool: aisdk.Tool{Name: ToolNameRenewWorkspaceExecutionSession, Description: "Extend an open execution session's lease to an explicit later time. Does not reopen preservation or deletion and does not change retention. Requires execution read/update and existing workspace access permissions.", Schema: func() aisdk.Schema {
		properties := executionControlProperties()
		properties["lease_expires_at"] = map[string]any{"type": "string", "format": "date-time", "description": "Explicit new expiration, later than both now and the current lease."}
		return aisdk.Schema{Properties: properties, Required: []string{"organization_id", "session_id", "expected_revision", "lease_expires_at"}}
	}()},
	MCPAnnotations: mcpMutationAnnotations,
	Handler: func(ctx context.Context, deps Deps, args RenewWorkspaceExecutionArgs) (codersdk.WorkspaceExecutionControlReceipt, error) {
		org, err := resolveOrganization(ctx, deps, args.OrganizationID)
		if err != nil {
			return codersdk.WorkspaceExecutionControlReceipt{}, err
		}
		return deps.coderClient.RenewWorkspaceExecutionSession(ctx, org, args.SessionID, codersdk.RenewWorkspaceExecutionRequest{ExpectedRevision: args.ExpectedRevision, LeaseExpiresAt: args.LeaseExpiresAt})
	},
}

func executionRevisionTool(name, description string, call func(context.Context, *codersdk.Client, uuid.UUID, uuid.UUID, codersdk.WorkspaceExecutionControlRequest) (codersdk.WorkspaceExecutionControlReceipt, error)) Tool[WorkspaceExecutionControlArgs, codersdk.WorkspaceExecutionControlReceipt] {
	properties := executionControlProperties()
	annotations := mcpMutationAnnotations
	if name == ToolNameRetryWorkspaceExecutionSession {
		annotations = mcpDestructiveAnnotations
		properties["artifact_expires_at"] = map[string]any{"type": "string", "format": "date-time", "description": "Explicit recovery extension later than now, the lease, and current finite artifact expiry. Creates a new closed preservation generation; original declarations and old artifacts remain immutable."}
	}
	return Tool[WorkspaceExecutionControlArgs, codersdk.WorkspaceExecutionControlReceipt]{
		Tool: aisdk.Tool{Name: name, Description: description, Schema: aisdk.Schema{Properties: properties, Required: []string{"organization_id", "session_id", "expected_revision"}}}, MCPAnnotations: annotations,
		Handler: func(ctx context.Context, deps Deps, args WorkspaceExecutionControlArgs) (codersdk.WorkspaceExecutionControlReceipt, error) {
			org, err := resolveOrganization(ctx, deps, args.OrganizationID)
			if err != nil {
				return codersdk.WorkspaceExecutionControlReceipt{}, err
			}
			request := codersdk.WorkspaceExecutionControlRequest{ExpectedRevision: args.ExpectedRevision}
			if name == ToolNameRetryWorkspaceExecutionSession {
				request.ArtifactExpiresAt = args.ArtifactExpiresAt
			}
			return call(ctx, deps.coderClient, org, args.SessionID, request)
		},
	}
}

// RetainWorkspaceExecutionSession explicitly protects a workspace from cleanup.
var RetainWorkspaceExecutionSession = executionRevisionTool(ToolNameRetainWorkspaceExecutionSession, "Retain an execution session's workspace and invalidate unfinished preservation. Cannot undo an already committed delete build. Requires execution read/update and existing workspace access permissions.", func(ctx context.Context, client *codersdk.Client, org, session uuid.UUID, req codersdk.WorkspaceExecutionControlRequest) (codersdk.WorkspaceExecutionControlReceipt, error) {
	return client.RetainWorkspaceExecutionSession(ctx, org, session, req)
})

// RetryWorkspaceExecutionSession requests reconciliation of a failed cleanup.
var RetryWorkspaceExecutionSession = executionRevisionTool(ToolNameRetryWorkspaceExecutionSession, "Request another attempt for preservation_failed or deletion_failed sessions, or recover expired/deleted results of a preserved session. Expired finite retention requires an explicit later artifact_expires_at. Deleted indefinite artifacts can be recollected without choosing an expiry. Clears the retry delay; the receipt remains failed until the controller observes progress. Does not itself declare success or start a delete build. Requires execution read/update and existing workspace access permissions.", func(ctx context.Context, client *codersdk.Client, org, session uuid.UUID, req codersdk.WorkspaceExecutionControlRequest) (codersdk.WorkspaceExecutionControlReceipt, error) {
	return client.RetryWorkspaceExecutionSession(ctx, org, session, req)
})
