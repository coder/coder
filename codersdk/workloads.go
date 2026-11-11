package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// WorkloadShapeEnv marks a template-declared agent as a shape: a daemon
// definition that is never started itself and is cloned for each runtime
// workload. The template sets it in the agent's env.
const WorkloadShapeEnv = "CODER_WORKLOAD_SHAPE"

// WorkloadIdentityKind says how a workspace daemon came to exist.
type WorkloadIdentityKind string

const (
	// WorkloadIdentityKindDeclared is a daemon declared by the template and
	// created by a workspace build.
	WorkloadIdentityKindDeclared WorkloadIdentityKind = "declared"
	// WorkloadIdentityKindRuntime is a daemon provisioned at runtime from a
	// shape.
	WorkloadIdentityKindRuntime WorkloadIdentityKind = "runtime"
)

// WorkloadAttestation says what a daemon's identity rests on.
type WorkloadAttestation string

const (
	// WorkloadAttestationAgentToken means the daemon holds a Coder-issued
	// agent token. Anything that holds the token is the daemon.
	WorkloadAttestationAgentToken WorkloadAttestation = "agent-token"
	// WorkloadAttestationKubernetesPod means the daemon proves it runs in a
	// specific pod with a token issued by that pod's cluster.
	WorkloadAttestationKubernetesPod WorkloadAttestation = "kubernetes-pod"
)

// WorkloadPod identifies the pod a runtime daemon runs in.
type WorkloadPod struct {
	Namespace      string `json:"namespace"`
	Name           string `json:"name"`
	UID            string `json:"uid"`
	ServiceAccount string `json:"service_account,omitempty"`
}

// WorkloadIdentity describes who a workspace daemon is.
type WorkloadIdentity struct {
	// ID is unique per daemon. For a runtime daemon it is the workspace ID
	// and the pod UID; for a declared daemon, the workspace ID and agent ID.
	ID            string               `json:"id"`
	Kind          WorkloadIdentityKind `json:"kind"`
	Attestation   WorkloadAttestation  `json:"attestation"`
	AgentID       uuid.UUID            `json:"agent_id" format:"uuid"`
	AgentName     string               `json:"agent_name"`
	WorkspaceID   uuid.UUID            `json:"workspace_id" format:"uuid"`
	WorkspaceName string               `json:"workspace_name"`
	// OwnerID is the user the daemon is associated with: the workspace owner.
	OwnerID       uuid.UUID `json:"owner_id" format:"uuid"`
	OwnerUsername string    `json:"owner_username"`
	// Shape is the name of the template-declared agent a runtime daemon was
	// cloned from.
	Shape string `json:"shape,omitempty"`
	// RequestedByID is the user who asked for a runtime daemon.
	RequestedByID       uuid.UUID    `json:"requested_by_id,omitempty" format:"uuid"`
	RequestedByUsername string       `json:"requested_by_username,omitempty"`
	Pod                 *WorkloadPod `json:"pod,omitempty"`
	// IdentityID is the workload identity that permissions attach to. A
	// declared daemon has none: it acts with its agent token.
	IdentityID uuid.UUID `json:"identity_id,omitempty" format:"uuid"`
	// SponsorID is the user account whose permissions bound what the
	// workload may do.
	SponsorID       uuid.UUID `json:"sponsor_id,omitempty" format:"uuid"`
	SponsorUsername string    `json:"sponsor_username,omitempty"`
	// Scopes narrow the sponsor's permissions. An empty list grants nothing.
	Scopes []APIKeyScope `json:"scopes,omitempty"`
}

// WorkloadStatus is the lifecycle of a runtime workload.
type WorkloadStatus string

const (
	// WorkloadStatusPending means no pod has been reported yet.
	WorkloadStatusPending WorkloadStatus = "pending"
	// WorkloadStatusStarting means the pod exists and its daemon has not
	// connected.
	WorkloadStatusStarting WorkloadStatus = "starting"
	// WorkloadStatusRunning means the daemon is connected.
	WorkloadStatusRunning WorkloadStatus = "running"
	// WorkloadStatusDisconnected means the daemon connected and then lost
	// its connection.
	WorkloadStatusDisconnected WorkloadStatus = "disconnected"
)

// Workload is a workspace daemon provisioned at runtime.
type Workload struct {
	// ID is also the ID of the workload's workspace agent once it exists.
	ID                  uuid.UUID      `json:"id" format:"uuid"`
	Name                string         `json:"name"`
	WorkspaceID         uuid.UUID      `json:"workspace_id" format:"uuid"`
	Shape               string         `json:"shape"`
	Status              WorkloadStatus `json:"status"`
	Command             string         `json:"command,omitempty"`
	CreatedAt           time.Time      `json:"created_at" format:"date-time"`
	RequestedByID       uuid.UUID      `json:"requested_by_id" format:"uuid"`
	RequestedByUsername string         `json:"requested_by_username"`
	// Identity is set once the pod exists.
	Identity string       `json:"identity,omitempty"`
	Pod      *WorkloadPod `json:"pod,omitempty"`
	// IdentityID is the workload identity that permissions attach to. It
	// exists from the moment the workload is requested.
	IdentityID uuid.UUID `json:"identity_id" format:"uuid"`
	// SponsorID is the user account whose permissions bound what the
	// workload may do.
	SponsorID       uuid.UUID `json:"sponsor_id" format:"uuid"`
	SponsorUsername string    `json:"sponsor_username"`
	// Scopes narrow the sponsor's permissions. An empty list grants nothing.
	Scopes []APIKeyScope `json:"scopes"`
}

// CreateWorkloadRequest asks for a new runtime workload in a workspace.
type CreateWorkloadRequest struct {
	// Name is optional. It must be a valid agent name.
	Name string `json:"name,omitempty"`
	// Shape selects the template-declared shape by agent name. It may be
	// empty when the workspace has exactly one shape.
	Shape string `json:"shape,omitempty"`
	// Command is run by the new daemon once it starts.
	Command string `json:"command,omitempty"`
	// Scopes are the API key scopes the workload may use on the sponsor's
	// behalf. Without any, the workload can do nothing in Coder.
	Scopes []APIKeyScope `json:"scopes,omitempty"`
}

// WorkloadOperatorShape is a shape an operator found in its namespace.
type WorkloadOperatorShape struct {
	// Deployment is the name of the Deployment that holds the pod template.
	Deployment string `json:"deployment"`
	// AgentToken is the shape agent's token from the pod template. It proves
	// that the Deployment belongs to that agent's workspace build.
	AgentToken string `json:"agent_token"`
}

// WorkloadOperatorPod is a pod in the operator's namespace that carries the
// operator's workload labels.
type WorkloadOperatorPod struct {
	WorkloadID     uuid.UUID `json:"workload_id" format:"uuid"`
	Name           string    `json:"name"`
	UID            string    `json:"uid"`
	ServiceAccount string    `json:"service_account"`
	Phase          string    `json:"phase"`
	// Created is true when this operator process created the pod itself.
	// Labels can be set by anyone who can create pods, so coderd only binds
	// a workload's identity to a pod the operator vouches for this way.
	Created bool `json:"created"`
}

// WorkloadOperatorSyncRequest is the operator's full view of its namespace.
// The operator sends it periodically.
type WorkloadOperatorSyncRequest struct {
	Namespace string `json:"namespace"`
	// Issuer, Audience and JWKS let coderd verify pod tokens from the
	// operator's cluster without access to that cluster.
	Issuer   string                  `json:"issuer"`
	Audience string                  `json:"audience"`
	JWKS     json.RawMessage         `json:"jwks"`
	Shapes   []WorkloadOperatorShape `json:"shapes"`
	Pods     []WorkloadOperatorPod   `json:"pods"`
}

// WorkloadOperatorShapeStatus reports whether coderd accepted a shape.
type WorkloadOperatorShapeStatus struct {
	Deployment  string    `json:"deployment"`
	AgentID     uuid.UUID `json:"agent_id,omitempty" format:"uuid"`
	AgentName   string    `json:"agent_name,omitempty"`
	WorkspaceID uuid.UUID `json:"workspace_id,omitempty" format:"uuid"`
	Error       string    `json:"error,omitempty"`
}

// WorkloadOperatorWorkload is a workload that should have a pod.
type WorkloadOperatorWorkload struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	Deployment  string    `json:"deployment"`
	WorkspaceID uuid.UUID `json:"workspace_id" format:"uuid"`
	// PodUID is the pod the workload's identity is bound to. It is empty
	// until coderd has accepted a pod the operator created.
	PodUID string `json:"pod_uid,omitempty"`
}

// WorkloadOperatorSyncResponse is the set of workloads the operator must
// keep running. The operator deletes its pods that are not listed.
type WorkloadOperatorSyncResponse struct {
	Shapes    []WorkloadOperatorShapeStatus `json:"shapes"`
	Workloads []WorkloadOperatorWorkload    `json:"workloads"`
}

// CreateWorkspaceWorkload provisions a runtime workload in a workspace.
func (c *Client) CreateWorkspaceWorkload(ctx context.Context, workspaceID uuid.UUID, req CreateWorkloadRequest) (Workload, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspaces/%s/workloads", workspaceID), req)
	if err != nil {
		return Workload{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return Workload{}, ReadBodyAsError(res)
	}
	var workload Workload
	return workload, ReadBodyAsJSON(res, &workload)
}

// WorkspaceWorkloads lists the runtime workloads of a workspace.
func (c *Client) WorkspaceWorkloads(ctx context.Context, workspaceID uuid.UUID) ([]Workload, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/workloads", workspaceID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var workloads []Workload
	return workloads, ReadBodyAsJSON(res, &workloads)
}

// DeleteWorkspaceWorkload removes a runtime workload and its daemon.
func (c *Client) DeleteWorkspaceWorkload(ctx context.Context, workspaceID, workloadID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/workspaces/%s/workloads/%s", workspaceID, workloadID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// SyncWorkloadOperator reports an operator's namespace to coderd and returns
// the workloads it must keep running.
func (c *Client) SyncWorkloadOperator(ctx context.Context, organizationID uuid.UUID, name string, req WorkloadOperatorSyncRequest) (WorkloadOperatorSyncResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/workload-operators/%s/sync", organizationID, name), req)
	if err != nil {
		return WorkloadOperatorSyncResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkloadOperatorSyncResponse{}, ReadBodyAsError(res)
	}
	var resp WorkloadOperatorSyncResponse
	return resp, ReadBodyAsJSON(res, &resp)
}
