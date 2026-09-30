package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// WorkspaceExecutionDeclarations are immutable preservation and cleanup inputs.
type WorkspaceExecutionDeclarations struct {
	ResultAgentName   string     `json:"result_agent_name,omitempty"`
	ResultAgentID     *uuid.UUID `json:"result_agent_id,omitempty" format:"uuid"`
	ResultPaths       []string   `json:"result_paths"`
	ExecutionDeadline *time.Time `json:"execution_deadline,omitempty" format:"date-time"`
	ArtifactExpiresAt *time.Time `json:"artifact_expires_at,omitempty" format:"date-time"`
}

// AcquireWorkspaceExecutionRequest binds one request to one workspace.
type AcquireWorkspaceExecutionRequest struct {
	RequestID      uuid.UUID                      `json:"request_id" format:"uuid"`
	OwnerID        uuid.UUID                      `json:"owner_id" format:"uuid"`
	WorkspaceID    uuid.UUID                      `json:"workspace_id,omitempty" format:"uuid"`
	Create         *CreateWorkspaceRequest        `json:"create,omitempty"`
	LeaseExpiresAt time.Time                      `json:"lease_expires_at" format:"date-time"`
	Disposable     bool                           `json:"disposable"`
	Retained       *bool                          `json:"retained"`
	Declarations   WorkspaceExecutionDeclarations `json:"declarations"`
}

// WorkspaceExecutionBuild reports the acquisition build's observed outcome.
type WorkspaceExecutionBuild struct {
	ID        uuid.UUID            `json:"id" format:"uuid"`
	Status    WorkspaceStatus      `json:"status"`
	JobStatus ProvisionerJobStatus `json:"job_status"`
	Error     string               `json:"error,omitempty"`
	ErrorCode string               `json:"error_code,omitempty"`
}

// WorkspaceExecutionSession is a durable acquisition and lifecycle receipt.
// State describes the session, not workspace readiness or command success.
type WorkspaceExecutionSession struct {
	ID                         uuid.UUID                      `json:"id" format:"uuid"`
	OrganizationID             uuid.UUID                      `json:"organization_id" format:"uuid"`
	OwnerID                    uuid.UUID                      `json:"owner_id" format:"uuid"`
	ActorID                    uuid.UUID                      `json:"actor_id" format:"uuid"`
	RequestID                  uuid.UUID                      `json:"request_id" format:"uuid"`
	WorkspaceID                *uuid.UUID                     `json:"workspace_id,omitempty" format:"uuid"`
	AcquisitionBuildID         *uuid.UUID                     `json:"acquisition_build_id,omitempty" format:"uuid"`
	AcquisitionBuild           *WorkspaceExecutionBuild       `json:"acquisition_build,omitempty"`
	EffectiveArtifactExpiresAt *time.Time                     `json:"effective_artifact_expires_at,omitempty" format:"date-time"`
	SourceUnavailable          bool                           `json:"source_unavailable"`
	State                      string                         `json:"state"`
	Disposable                 bool                           `json:"disposable"`
	Retained                   bool                           `json:"retained"`
	LeaseExpiresAt             time.Time                      `json:"lease_expires_at" format:"date-time"`
	Revision                   int64                          `json:"revision"`
	Declarations               WorkspaceExecutionDeclarations `json:"declarations"`
	Error                      string                         `json:"error,omitempty"`
}

// AcquireWorkspaceExecution creates or reuses the workspace bound to a request.
func (c *Client) AcquireWorkspaceExecution(ctx context.Context, org uuid.UUID, request AcquireWorkspaceExecutionRequest) (WorkspaceExecutionSession, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/workspace-executions", org), request)
	if err != nil {
		return WorkspaceExecutionSession{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		return WorkspaceExecutionSession{}, ReadBodyAsError(res)
	}
	var result WorkspaceExecutionSession
	err = ReadBodyAsJSON(res, &result)
	return result, err
}

// WorkspaceExecutionSession returns the receipt without renewing its lease.
func (c *Client) WorkspaceExecutionSession(ctx context.Context, org, id uuid.UUID) (WorkspaceExecutionSession, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/workspace-executions/%s", org, id), nil)
	if err != nil {
		return WorkspaceExecutionSession{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceExecutionSession{}, ReadBodyAsError(res)
	}
	var result WorkspaceExecutionSession
	err = ReadBodyAsJSON(res, &result)
	return result, err
}
