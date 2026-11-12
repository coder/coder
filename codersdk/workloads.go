package codersdk

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/websocket"
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
	// WorkloadTermEnv carries the terminal type for an interactive
	// workload's command past the program that starts it.
	WorkloadTermEnv = "CODER_WORKLOAD_TERM"
)

// WorkloadNetwork is what a workload's sandbox can reach over the network.
// Its channel to Coder works in every mode.
type WorkloadNetwork string

const (
	// WorkloadNetworkNone gives the sandbox no network at all. Coder,
	// through the channel, is the only thing it can talk to.
	WorkloadNetworkNone WorkloadNetwork = "none"
	// WorkloadNetworkOutbound gives the sandbox ordinary outbound network
	// access. It still cannot reach other sandboxes.
	WorkloadNetworkOutbound WorkloadNetwork = "outbound"
)

// WorkloadMount is a directory of the workspace that a workload's sandbox
// can see, at the same path.
type WorkloadMount struct {
	Path string `json:"path"`
	// Writable lets the workload change the directory. Without it the
	// workload can only read.
	Writable bool `json:"writable,omitempty"`
}

// String formats the mount the way the CLI accepts it.
func (m WorkloadMount) String() string {
	if m.Writable {
		return m.Path + ":rw"
	}
	return m.Path
}

// ParseWorkloadMount parses "path" or "path:rw".
func ParseWorkloadMount(raw string) WorkloadMount {
	if path, ok := strings.CutSuffix(raw, ":rw"); ok {
		return WorkloadMount{Path: path, Writable: true}
	}
	return WorkloadMount{Path: strings.TrimSuffix(raw, ":ro")}
}

// WorkloadEndReason says why a workload is over.
type WorkloadEndReason string

const (
	// WorkloadEndReasonExited means the command ran to its end.
	WorkloadEndReasonExited WorkloadEndReason = "exited"
	// WorkloadEndReasonRemoved means a user removed the workload.
	WorkloadEndReasonRemoved WorkloadEndReason = "removed"
	// WorkloadEndReasonStopped means the workspace daemon shut down, as it
	// does when the workspace stops.
	WorkloadEndReasonStopped WorkloadEndReason = "stopped"
	// WorkloadEndReasonLost means the sandbox disappeared: it was killed
	// from outside, or its daemon went away without saying so.
	WorkloadEndReasonLost WorkloadEndReason = "lost"
	// WorkloadEndReasonFailed means the sandbox never started.
	WorkloadEndReasonFailed WorkloadEndReason = "failed"
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
	// WorkloadAttestationKubernetesPod means a cluster confirmed which pod
	// the identity's daemon runs in, from the token the kubelet obtained
	// for that pod. A workload with this attestation has the pod to
	// itself.
	WorkloadAttestationKubernetesPod WorkloadAttestation = "kubernetes-pod"
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
	// AttestationSubject is what was attested, when that is not Coder's
	// own credential: for a pod, the UID the cluster gave it.
	AttestationSubject string `json:"attestation_subject,omitempty"`
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

// WorkloadStatus is the lifecycle of a workload. A workload is live while
// it is starting, running or unknown. Every other status is final: the
// workload is over and its identity can no longer be used.
type WorkloadStatus string

const (
	// WorkloadStatusStarting means the daemon has not reported a sandbox
	// yet.
	WorkloadStatusStarting WorkloadStatus = "starting"
	// WorkloadStatusRunning means the workload's command is running.
	WorkloadStatusRunning WorkloadStatus = "running"
	// WorkloadStatusUnknown means the workspace daemon cannot be reached.
	WorkloadStatusUnknown WorkloadStatus = "unknown"
	// WorkloadStatusExited means the command ran to its end.
	WorkloadStatusExited WorkloadStatus = "exited"
	// WorkloadStatusRemoved means a user removed the workload.
	WorkloadStatusRemoved WorkloadStatus = "removed"
	// WorkloadStatusStopped means the workload ended with its workspace
	// daemon, as it does when the workspace stops.
	WorkloadStatusStopped WorkloadStatus = "stopped"
	// WorkloadStatusLost means the sandbox disappeared.
	WorkloadStatusLost WorkloadStatus = "lost"
	// WorkloadStatusFailed means the sandbox never started.
	WorkloadStatusFailed WorkloadStatus = "failed"
)

// Workload is a command that runs in a sandbox inside a workspace daemon,
// with an identity of its own.
type Workload struct {
	// ID is the workload's identity ID.
	ID          uuid.UUID `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	WorkspaceID uuid.UUID `json:"workspace_id" format:"uuid"`
	// WorkspaceName is set when workloads of several workspaces are
	// listed.
	WorkspaceName string `json:"workspace_name,omitempty"`
	// OwnWorkspace means the workspace was made for this workload and is
	// deleted when the workload is over.
	OwnWorkspace bool           `json:"own_workspace,omitempty"`
	AgentID      uuid.UUID      `json:"agent_id" format:"uuid"`
	AgentName    string         `json:"agent_name"`
	Status       WorkloadStatus `json:"status"`
	Command      string         `json:"command"`
	CreatedAt    time.Time      `json:"created_at" format:"date-time"`
	// FinishedAt is set once the workload is over.
	FinishedAt *time.Time `json:"finished_at,omitempty" format:"date-time"`
	// ExitCode is the command's exit code, when it is known.
	ExitCode *int `json:"exit_code,omitempty"`
	// Image, Network and Mounts are what the daemon's policy settled on.
	Image   string          `json:"image,omitempty"`
	Network WorkloadNetwork `json:"network,omitempty"`
	Mounts  []WorkloadMount `json:"mounts,omitempty"`
	// Identity is the workload's identity as it is displayed.
	Identity string `json:"identity"`
	// SponsorID is the user account whose permissions bound what the
	// workload may do.
	SponsorID       uuid.UUID `json:"sponsor_id" format:"uuid"`
	SponsorUsername string    `json:"sponsor_username"`
	// UserID is the service account that is this identity, for a workload
	// with a workspace to itself. The workload acts as it and owns the
	// workspace.
	UserID *uuid.UUID `json:"user_id,omitempty" format:"uuid"`
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
	// Image asks for another image than the daemon's default. The
	// template decides which images are allowed.
	Image string `json:"image,omitempty"`
	// Network asks for another network than the daemon's default. Asking
	// for none is always allowed.
	Network WorkloadNetwork `json:"network,omitempty"`
	// Mounts asks for directories of the workspace to be visible in the
	// sandbox. The template decides which directories may be shared.
	Mounts []WorkloadMount `json:"mounts,omitempty"`
	// Interactive gives the command a terminal that the sponsor can attach
	// to.
	Interactive bool `json:"interactive,omitempty"`
	// Scopes are the API key scopes the workload may use on the sponsor's
	// behalf. Without any, the workload can do nothing in Coder.
	Scopes []APIKeyScope `json:"scopes,omitempty"`
	// RemoveWorkspace deletes the workspace when the workload is over. It
	// is for a workspace that was made for this one workload.
	RemoveWorkspace bool `json:"remove_workspace,omitempty"`
}

// LaunchWorkloadRequest asks for a workload with a workspace to itself. The
// workload's identity owns the workspace, and the caller sponsors it.
type LaunchWorkloadRequest struct {
	CreateWorkloadRequest
	// TemplateID is the template the workspace is made from. The caller
	// must be allowed to use it.
	TemplateID uuid.UUID `json:"template_id" format:"uuid"`
	// TemplateVersionPresetID selects a preset of the template's active
	// version. A workspace that was built ahead of time for the preset is
	// claimed when there is one.
	TemplateVersionPresetID uuid.UUID `json:"template_version_preset_id,omitempty" format:"uuid"`
	// WorkspaceName is optional. The workspace is named like the workload
	// otherwise.
	WorkspaceName string `json:"workspace_name,omitempty"`
	// Keep leaves the workspace in place when the workload is over.
	Keep bool `json:"keep,omitempty"`
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
	// Image, Network and Mounts are requests. The daemon's policy, which
	// the template sets, decides whether to honor them.
	Image   string          `json:"image,omitempty"`
	Network WorkloadNetwork `json:"network,omitempty"`
	Mounts  []WorkloadMount `json:"mounts,omitempty"`
	// Interactive gives the command a terminal.
	Interactive bool `json:"interactive,omitempty"`
}

// AgentWorkloadState is a workload as its workspace daemon sees it.
type AgentWorkloadState struct {
	ID      uuid.UUID       `json:"id" format:"uuid"`
	Name    string          `json:"name"`
	Sandbox WorkloadSandbox `json:"sandbox"`
	// Image, Network and Mounts are what the daemon's policy settled on.
	Image     string          `json:"image"`
	Network   WorkloadNetwork `json:"network"`
	Mounts    []WorkloadMount `json:"mounts,omitempty"`
	Running   bool            `json:"running"`
	ExitCode  *int            `json:"exit_code,omitempty"`
	StartedAt time.Time       `json:"started_at" format:"date-time"`
}

// AgentWorkloadStart is what a workspace daemon reports once it has tried
// to start a workload that coderd gave it.
type AgentWorkloadStart struct {
	// State is set when the workload runs.
	State *AgentWorkloadState `json:"state,omitempty"`
	// Error says why it does not.
	Error string `json:"error,omitempty"`
}

// AgentWorkloadExit is what a workspace daemon reports when a workload is
// over.
type AgentWorkloadExit struct {
	// ExitCode is nil when the daemon does not know it.
	ExitCode *int              `json:"exit_code,omitempty"`
	Reason   WorkloadEndReason `json:"reason"`
	// OutputTail is the end of the workload's output.
	OutputTail string `json:"output_tail,omitempty"`
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

// LaunchWorkload runs a command in a workspace that is made for it and
// that its identity owns.
func (c *Client) LaunchWorkload(ctx context.Context, req LaunchWorkloadRequest) (Workload, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/workloads", req)
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

// WorkspaceWorkloadsOptions selects which workloads to list.
type WorkspaceWorkloadsOptions struct {
	// Finished also lists the most recent workloads that are over.
	Finished bool
}

// WorkspaceWorkloads lists the workloads of a workspace: the live ones,
// oldest first, followed by finished ones when asked, newest first.
func (c *Client) WorkspaceWorkloads(ctx context.Context, workspaceID uuid.UUID, opts WorkspaceWorkloadsOptions) ([]Workload, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/workloads", workspaceID), nil,
		func(r *http.Request) {
			if opts.Finished {
				q := r.URL.Query()
				q.Set("finished", "true")
				r.URL.RawQuery = q.Encode()
			}
		})
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

// AttachWorkspaceWorkload connects to the terminal of an interactive
// workload. The connection carries what a reconnecting PTY carries: JSON
// messages with input and window sizes one way, raw output the other.
func (c *Client) AttachWorkspaceWorkload(ctx context.Context, workspaceID, workloadID uuid.UUID, height, width uint16) (net.Conn, error) {
	serverURL, err := c.URL.Parse(fmt.Sprintf("/api/v2/workspaces/%s/workloads/%s/attach", workspaceID, workloadID))
	if err != nil {
		return nil, xerrors.Errorf("parse url: %w", err)
	}
	q := serverURL.Query()
	q.Set("height", strconv.Itoa(int(height)))
	q.Set("width", strconv.Itoa(int(width)))
	serverURL.RawQuery = q.Encode()

	// Non-browser websocket auth uses the session token header. A cookie
	// jar is not used.
	httpClient := *c.HTTPClient
	httpClient.Jar = nil
	headers := http.Header{}
	headers.Set(SessionTokenHeader, c.SessionToken())
	//nolint:bodyclose // The websocket owns the response body.
	conn, res, err := websocket.Dial(ctx, serverURL.String(), &websocket.DialOptions{
		HTTPClient: &httpClient,
		HTTPHeader: headers,
	})
	if err != nil {
		if res == nil {
			return nil, err
		}
		return nil, ReadBodyAsError(res)
	}
	return websocket.NetConn(context.Background(), conn, websocket.MessageBinary), nil
}

// Workloads lists the workloads the caller sponsors, in every workspace:
// the live ones, oldest first, followed by finished ones when asked, newest
// first. The workspace of a finished workload may be gone.
func (c *Client) Workloads(ctx context.Context, opts WorkspaceWorkloadsOptions) ([]Workload, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/workloads", nil,
		func(r *http.Request) {
			if opts.Finished {
				q := r.URL.Query()
				q.Set("finished", "true")
				r.URL.RawQuery = q.Encode()
			}
		})
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
