package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// WorkspaceExecutionControlRequest fences a mutation to an observed revision.
type WorkspaceExecutionControlRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
	// LeaseExpiresAt applies only to renewal.
	LeaseExpiresAt time.Time `json:"lease_expires_at,omitempty" format:"date-time"`
	// ArtifactExpiresAt applies only to retry and explicitly extends finite retention.
	ArtifactExpiresAt *time.Time `json:"artifact_expires_at,omitempty" format:"date-time"`
}

// RenewWorkspaceExecutionRequest extends a session's explicit lease.
type RenewWorkspaceExecutionRequest struct {
	ExpectedRevision int64     `json:"expected_revision"`
	LeaseExpiresAt   time.Time `json:"lease_expires_at" format:"date-time"`
}

// WorkspaceExecutionControlReceipt reports the committed lifecycle state.
type WorkspaceExecutionControlReceipt struct {
	ID                         uuid.UUID  `json:"id" format:"uuid"`
	State                      string     `json:"state"`
	Revision                   int64      `json:"revision"`
	Retained                   bool       `json:"retained"`
	LeaseExpiresAt             time.Time  `json:"lease_expires_at" format:"date-time"`
	Error                      string     `json:"error,omitempty"`
	EffectiveArtifactExpiresAt *time.Time `json:"effective_artifact_expires_at,omitempty" format:"date-time"`
}

func (c *Client) workspaceExecutionControl(ctx context.Context, org, session uuid.UUID, action string, request any) (WorkspaceExecutionControlReceipt, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/workspace-executions/%s/%s", org, session, action), request)
	if err != nil {
		return WorkspaceExecutionControlReceipt{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceExecutionControlReceipt{}, ReadBodyAsError(res)
	}
	var result WorkspaceExecutionControlReceipt
	err = ReadBodyAsJSON(res, &result)
	return result, err
}

// RenewWorkspaceExecutionSession extends an open session's lease without changing retention.
func (c *Client) RenewWorkspaceExecutionSession(ctx context.Context, org, session uuid.UUID, request RenewWorkspaceExecutionRequest) (WorkspaceExecutionControlReceipt, error) {
	return c.workspaceExecutionControl(ctx, org, session, "renew", request)
}

// RetainWorkspaceExecutionSession protects the workspace unless deletion has committed.
func (c *Client) RetainWorkspaceExecutionSession(ctx context.Context, org, session uuid.UUID, request WorkspaceExecutionControlRequest) (WorkspaceExecutionControlReceipt, error) {
	return c.workspaceExecutionControl(ctx, org, session, "retain", request)
}

// RetryWorkspaceExecutionSession requests another attempt at a recoverable failure.
func (c *Client) RetryWorkspaceExecutionSession(ctx context.Context, org, session uuid.UUID, request WorkspaceExecutionControlRequest) (WorkspaceExecutionControlReceipt, error) {
	return c.workspaceExecutionControl(ctx, org, session, "retry", request)
}
