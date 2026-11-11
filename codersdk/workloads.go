package codersdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

const (
	// WorkloadIdentityHeader is set by a workspace daemon on a request it
	// forwards for one of its workloads. It names the workload identity the
	// request belongs to. coderd honors it only together with that daemon's
	// own credential, and only for an identity assigned to that daemon.
	WorkloadIdentityHeader = "Coder-Workload-Identity"
	// WorkloadSocketEnv is set inside a workload's sandbox. It is the path
	// of a unix socket that leads to Coder's API as that workload. The
	// socket is the workload's identity: the sandbox holds no credential.
	WorkloadSocketEnv = "CODER_WORKLOAD_SOCKET"
)

// WorkloadIdentityKind says what kind of principal a process acts as.
type WorkloadIdentityKind string

const (
	// WorkloadIdentityKindDaemon is a workspace daemon declared by the
	// template.
	WorkloadIdentityKindDaemon WorkloadIdentityKind = "daemon"
	// WorkloadIdentityKindWorkload is a workload started at runtime inside
	// a workspace daemon's sandbox.
	WorkloadIdentityKindWorkload WorkloadIdentityKind = "workload"
)

// WorkloadAttestation says what an identity rests on.
type WorkloadAttestation string

const (
	// WorkloadAttestationAgentToken means the process holds a Coder-issued
	// agent token. Anything that holds the token is the daemon.
	WorkloadAttestationAgentToken WorkloadAttestation = "agent-token"
	// WorkloadAttestationWorkspaceDaemon means the workspace daemon vouches
	// for the workload: it created the sandbox and it alone connects the
	// sandbox to Coder. The workload holds no credential.
	WorkloadAttestationWorkspaceDaemon WorkloadAttestation = "workspace-daemon"
)

// WorkloadSandbox is where a workload runs inside its workspace daemon.
type WorkloadSandbox struct {
	// Runtime is the isolation backend, for example "docker".
	Runtime string `json:"runtime"`
	// ID identifies the sandbox to that runtime.
	ID string `json:"id"`
}

// WorkloadIdentity describes who a process in a workspace is to Coder.
type WorkloadIdentity struct {
	// ID is unique per principal: the workspace ID and the workload
	// identity ID, or for a daemon the workspace ID and the agent ID.
	ID          string               `json:"id"`
	Kind        WorkloadIdentityKind `json:"kind"`
	Attestation WorkloadAttestation  `json:"attestation"`
	// AgentID is the workspace daemon: the process itself, or the daemon
	// that runs and vouches for the workload.
	AgentID       uuid.UUID `json:"agent_id" format:"uuid"`
	AgentName     string    `json:"agent_name"`
	WorkspaceID   uuid.UUID `json:"workspace_id" format:"uuid"`
	WorkspaceName string    `json:"workspace_name"`
	OwnerID       uuid.UUID `json:"owner_id" format:"uuid"`
	OwnerUsername string    `json:"owner_username"`
	// IdentityID is the workload identity that permissions attach to. A
	// daemon has none: it acts with its agent token.
	IdentityID uuid.UUID `json:"identity_id,omitempty" format:"uuid"`
	// Name is the workload's name.
	Name string `json:"name,omitempty"`
	// SponsorID is the user account whose permissions bound what the
	// workload may do.
	SponsorID       uuid.UUID `json:"sponsor_id,omitempty" format:"uuid"`
	SponsorUsername string    `json:"sponsor_username,omitempty"`
	// Scopes narrow the sponsor's permissions. An empty list grants nothing.
	Scopes  []APIKeyScope    `json:"scopes,omitempty"`
	Sandbox *WorkloadSandbox `json:"sandbox,omitempty"`
}

// WorkloadStatus is the lifecycle of a workload.
type WorkloadStatus string

const (
	// WorkloadStatusStarting means the daemon has not reported a sandbox
	// yet.
	WorkloadStatusStarting WorkloadStatus = "starting"
	// WorkloadStatusRunning means the workload's command is running.
	WorkloadStatusRunning WorkloadStatus = "running"
	// WorkloadStatusExited means the command has finished. The identity
	// can no longer be used.
	WorkloadStatusExited WorkloadStatus = "exited"
	// WorkloadStatusUnknown means the workspace daemon cannot be reached.
	WorkloadStatusUnknown WorkloadStatus = "unknown"
)

// Workload is a command that runs in a sandbox inside a workspace daemon,
// with an identity of its own.
type Workload struct {
	// ID is the workload's identity ID.
	ID          uuid.UUID      `json:"id" format:"uuid"`
	Name        string         `json:"name"`
	WorkspaceID uuid.UUID      `json:"workspace_id" format:"uuid"`
	AgentID     uuid.UUID      `json:"agent_id" format:"uuid"`
	AgentName   string         `json:"agent_name"`
	Status      WorkloadStatus `json:"status"`
	ExitCode    *int           `json:"exit_code,omitempty"`
	Command     string         `json:"command"`
	CreatedAt   time.Time      `json:"created_at" format:"date-time"`
	// Identity is the workload's identity as it is displayed.
	Identity string `json:"identity"`
	// SponsorID is the user account whose permissions bound what the
	// workload may do.
	SponsorID       uuid.UUID `json:"sponsor_id" format:"uuid"`
	SponsorUsername string    `json:"sponsor_username"`
	// Scopes narrow the sponsor's permissions. An empty list grants nothing.
	Scopes  []APIKeyScope    `json:"scopes"`
	Sandbox *WorkloadSandbox `json:"sandbox,omitempty"`
}

// CreateWorkloadRequest asks for a workload in a workspace.
type CreateWorkloadRequest struct {
	// Name is optional. It must be a valid agent name.
	Name string `json:"name,omitempty"`
	// Agent selects the workspace daemon by name. It may be empty when the
	// workspace has exactly one.
	Agent string `json:"agent,omitempty"`
	// Command is run by a shell inside the sandbox.
	Command string `json:"command"`
	// Image overrides the image the daemon uses for sandboxes.
	Image string `json:"image,omitempty"`
	// Scopes are the API key scopes the workload may use on the sponsor's
	// behalf. Without any, the workload can do nothing in Coder.
	Scopes []APIKeyScope `json:"scopes,omitempty"`
}

// WorkloadLogEvent is one item of a workload's output stream. The last
// event carries the exit code and no output.
type WorkloadLogEvent struct {
	Output   string `json:"output,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
}

// AgentWorkloadSpec is what coderd asks a workspace daemon to run.
type AgentWorkloadSpec struct {
	// ID is the workload identity the daemon vouches for when it forwards
	// the sandbox's requests.
	ID      uuid.UUID `json:"id" format:"uuid"`
	Name    string    `json:"name"`
	Command string    `json:"command"`
	Image   string    `json:"image,omitempty"`
}

// AgentWorkloadState is a workload as its workspace daemon sees it.
type AgentWorkloadState struct {
	ID        uuid.UUID       `json:"id" format:"uuid"`
	Name      string          `json:"name"`
	Sandbox   WorkloadSandbox `json:"sandbox"`
	Running   bool            `json:"running"`
	ExitCode  *int            `json:"exit_code,omitempty"`
	StartedAt time.Time       `json:"started_at" format:"date-time"`
}

// CreateWorkspaceWorkload runs a command in a new sandbox of a workspace.
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

// WorkspaceWorkloads lists the workloads of a workspace.
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

// DeleteWorkspaceWorkload revokes a workload's identity and stops it.
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

// WorkspaceWorkloadLogs follows a workload's output until its command
// exits, calling handle for every event. The last event has the exit code.
func (c *Client) WorkspaceWorkloadLogs(ctx context.Context, workspaceID, workloadID uuid.UUID, handle func(WorkloadLogEvent) error) error {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/workloads/%s/logs", workspaceID, workloadID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return DecodeWorkloadLogEvents(res.Body, handle)
}

// DecodeWorkloadLogEvents reads newline-delimited log events.
func DecodeWorkloadLogEvents(r io.Reader, handle func(WorkloadLogEvent) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var event WorkloadLogEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return xerrors.Errorf("decode log event: %w", err)
		}
		if err := handle(event); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// WorkloadIdentity returns the identity coderd sees for the caller. It
// works for a workspace daemon and for a workload inside one.
func (c *Client) WorkloadIdentity(ctx context.Context) (WorkloadIdentity, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/workspaceagents/me/identity", nil)
	if err != nil {
		return WorkloadIdentity{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkloadIdentity{}, ReadBodyAsError(res)
	}
	var identity WorkloadIdentity
	return identity, ReadBodyAsJSON(res, &identity)
}
