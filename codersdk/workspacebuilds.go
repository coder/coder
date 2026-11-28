package codersdk

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type WorkspaceTransition string

const (
	WorkspaceTransitionStart  WorkspaceTransition = "start"
	WorkspaceTransitionStop   WorkspaceTransition = "stop"
	WorkspaceTransitionDelete WorkspaceTransition = "delete"
)

func WorkspaceTransitionEnums() []WorkspaceTransition {
	return []WorkspaceTransition{WorkspaceTransitionStart, WorkspaceTransitionStop, WorkspaceTransitionDelete}
}

type WorkspaceStatus string

const (
	WorkspaceStatusPending   WorkspaceStatus = "pending"
	WorkspaceStatusStarting  WorkspaceStatus = "starting"
	WorkspaceStatusRunning   WorkspaceStatus = "running"
	WorkspaceStatusStopping  WorkspaceStatus = "stopping"
	WorkspaceStatusStopped   WorkspaceStatus = "stopped"
	WorkspaceStatusFailed    WorkspaceStatus = "failed"
	WorkspaceStatusCanceling WorkspaceStatus = "canceling"
	WorkspaceStatusCanceled  WorkspaceStatus = "canceled"
	WorkspaceStatusDeleting  WorkspaceStatus = "deleting"
	WorkspaceStatusDeleted   WorkspaceStatus = "deleted"
)

type BuildReason string

const (
	// BuildReasonInitiator "initiator" is used when a workspace build is triggered by a user.
	// Combined with the initiator id/username, it indicates which user initiated the build.
	BuildReasonInitiator BuildReason = "initiator"
	// BuildReasonAutostart "autostart" is used when a build to start a workspace is triggered by Autostart.
	// The initiator id/username in this case is the workspace owner and can be ignored.
	BuildReasonAutostart BuildReason = "autostart"
	// BuildReasonAutostop "autostop" is used when a build to stop a workspace is triggered by Autostop.
	// The initiator id/username in this case is the workspace owner and can be ignored.
	BuildReasonAutostop BuildReason = "autostop"
	// BuildReasonDormancy "dormancy" is used when a build to stop a workspace is triggered due to inactivity (dormancy).
	// The initiator id/username in this case is the workspace owner and can be ignored.
	BuildReasonDormancy BuildReason = "dormancy"
	// BuildReasonDashboard "dashboard" is used when a build to start a workspace is triggered by the dashboard.
	BuildReasonDashboard BuildReason = "dashboard"
	// BuildReasonCLI "cli" is used when a build to start a workspace is triggered by the CLI.
	BuildReasonCLI BuildReason = "cli"
	// BuildReasonSSHConnection "ssh_connection" is used when a build to start a workspace is triggered by an SSH connection.
	BuildReasonSSHConnection BuildReason = "ssh_connection"
	// BuildReasonVSCodeConnection "vscode_connection" is used when a build to start a workspace is triggered by a VS Code connection.
	BuildReasonVSCodeConnection BuildReason = "vscode_connection"
	// BuildReasonJetbrainsConnection "jetbrains_connection" is used when a build to start a workspace is triggered by a JetBrains connection.
	BuildReasonJetbrainsConnection BuildReason = "jetbrains_connection"
	// BuildReasonAutodelete "autodelete" is used when a build to delete a workspace is triggered by dormancy auto-delete.
	BuildReasonAutodelete BuildReason = "autodelete"
)

// WorkspaceBuild is an at-point representation of a workspace state.
// BuildNumbers start at 1 and increase by 1 for each subsequent build
type WorkspaceBuild struct {
	ID               uuid.UUID `json:"id" format:"uuid" wsrelated:"latest_build"`
	CreatedAt        time.Time `json:"created_at" format:"date-time" wsrelated:"latest_build"`
	UpdatedAt        time.Time `json:"updated_at" format:"date-time" wsrelated:"latest_build"`
	WorkspaceID      uuid.UUID `json:"workspace_id" format:"uuid" wsrelated:"latest_build"`
	WorkspaceName    string    `json:"workspace_name" wsrelated:"latest_build"`
	WorkspaceOwnerID uuid.UUID `json:"workspace_owner_id" format:"uuid" wsrelated:"latest_build"`
	// WorkspaceOwnerName is the username of the owner of the workspace.
	WorkspaceOwnerName      string               `json:"workspace_owner_name" wsrelated:"latest_build"`
	WorkspaceOwnerAvatarURL string               `json:"workspace_owner_avatar_url,omitempty" wsrelated:"latest_build"`
	TemplateVersionID       uuid.UUID            `json:"template_version_id" format:"uuid" wsrelated:"latest_build"`
	TemplateVersionName     string               `json:"template_version_name" wsrelated:"latest_build.template_version"`
	BuildNumber             int32                `json:"build_number" wsrelated:"latest_build"`
	Transition              WorkspaceTransition  `json:"transition" wsrelated:"latest_build"`
	InitiatorID             uuid.UUID            `json:"initiator_id" format:"uuid" wsrelated:"latest_build"`
	InitiatorUsername       string               `json:"initiator_name" wsrelated:"latest_build"`
	Job                     ProvisionerJob       `json:"job" wsrelated:"latest_build.job"`
	Reason                  BuildReason          `db:"reason" json:"reason" wsrelated:"latest_build"`
	Resources               []WorkspaceResource  `json:"resources" wsrelated:"latest_build.resources"`
	Deadline                NullTime             `json:"deadline,omitempty" format:"date-time" wsrelated:"latest_build"`
	MaxDeadline             NullTime             `json:"max_deadline,omitempty" format:"date-time" wsrelated:"latest_build"`
	Status                  WorkspaceStatus      `json:"status" wsrelated:"latest_build.job"`
	DailyCost               int32                `json:"daily_cost" wsrelated:"latest_build"`
	MatchedProvisioners     *MatchedProvisioners `json:"matched_provisioners,omitempty" wsrelated:"latest_build.job"`
	TemplateVersionPresetID *uuid.UUID           `json:"template_version_preset_id" format:"uuid" wsrelated:"latest_build"`
	HasExternalAgent        *bool                `json:"has_external_agent,omitempty" wsrelated:"latest_build"`
	// DeadlineChange explains the last change to Deadline. Servers that predate
	// it omit it.
	DeadlineChange *WorkspaceBuildDeadlineChange `json:"deadline_change,omitempty" wsrelated:"latest_build"`
}

// WorkspaceBuildDeadlineChange explains the last change to a build's deadline.
type WorkspaceBuildDeadlineChange struct {
	// Reason is why the deadline last changed. Treat unrecognized values as
	// unknown.
	Reason WorkspaceBuildDeadlineReason `json:"reason"`
	// ChangedAt is when the deadline last changed. It is null when the time is
	// unknown.
	ChangedAt *time.Time `json:"changed_at" format:"date-time"`
	// ChangedBy is the user who made the change. It is null when no user made
	// it or the user is unknown.
	ChangedBy *MinimalUser `json:"changed_by"`
	// Sources is the activity that last extended the deadline, busiest first.
	// It is empty unless Reason is "activity". Ignore unrecognized types.
	Sources []WorkspaceBuildDeadlineSource `json:"sources"`
}

// WorkspaceBuildDeadlineSourceType identifies activity that changed a deadline.
type WorkspaceBuildDeadlineSourceType string

const (
	// WorkspaceBuildDeadlineSourceTypeApp is an app. Its method gives how it
	// connects to the workspace, with the values of connection logs.
	WorkspaceBuildDeadlineSourceTypeApp WorkspaceBuildDeadlineSourceType = "app"
	// WorkspaceBuildDeadlineSourceTypeConnection is a connection that reported
	// no app.
	WorkspaceBuildDeadlineSourceTypeConnection WorkspaceBuildDeadlineSourceType = "connection"
	// WorkspaceBuildDeadlineSourceTypeChatHeartbeat is a Coder Agents chat
	// using the workspace.
	WorkspaceBuildDeadlineSourceTypeChatHeartbeat WorkspaceBuildDeadlineSourceType = "chat_heartbeat"
	// WorkspaceBuildDeadlineSourceTypeChatWorkspaceUpload is a Coder Agents
	// chat uploading a file to the workspace.
	WorkspaceBuildDeadlineSourceTypeChatWorkspaceUpload WorkspaceBuildDeadlineSourceType = "chat_workspace_upload"
)

// WorkspaceBuildDeadlineSource is one kind of activity that extended a
// deadline.
type WorkspaceBuildDeadlineSource struct {
	// Type is the kind of activity.
	Type WorkspaceBuildDeadlineSourceType `json:"type"`
	// Method is how an app source connects to the workspace. It is only set
	// for app sources.
	Method ConnectionLogMethod `json:"method,omitempty"`
	// AppName is the normalized app name, such as "ssh". It is only set for
	// app sources without a workspace app.
	AppName string `json:"app_name,omitempty"`
	// AppID is the workspace app. It is only set for app sources with the
	// workspace_app method.
	AppID *uuid.UUID `json:"app_id,omitempty" format:"uuid"`
	// AgentID is the agent that reported the activity. It is set for app and
	// connection sources.
	AgentID *uuid.UUID `json:"agent_id,omitempty" format:"uuid"`
	// DisplayName is a human-readable label, such as "VS Code". It is not a
	// stable identifier.
	DisplayName string `json:"display_name"`
}

// WorkspaceResource describes resources used to create a workspace, for instance:
// containers, images, volumes.
type WorkspaceResource struct {
	ID         uuid.UUID                   `json:"id" format:"uuid" wsrelated:"latest_build.resources"`
	CreatedAt  time.Time                   `json:"created_at" format:"date-time" wsrelated:"latest_build.resources"`
	JobID      uuid.UUID                   `json:"job_id" format:"uuid" wsrelated:"latest_build.resources"`
	Transition WorkspaceTransition         `json:"workspace_transition" wsrelated:"latest_build.resources"`
	Type       string                      `json:"type" wsrelated:"latest_build.resources"`
	Name       string                      `json:"name" wsrelated:"latest_build.resources"`
	Hide       bool                        `json:"hide" wsrelated:"latest_build.resources"`
	Icon       string                      `json:"icon" wsrelated:"latest_build.resources"`
	Agents     []WorkspaceAgent            `json:"agents,omitempty" wsrelated:"latest_build.resources.agents"`
	Metadata   []WorkspaceResourceMetadata `json:"metadata,omitempty" wsrelated:"latest_build.resources.metadata"`
	DailyCost  int32                       `json:"daily_cost" wsrelated:"latest_build.resources"`
}

// WorkspaceResourceMetadata annotates the workspace resource with custom key-value pairs.
type WorkspaceResourceMetadata struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Sensitive bool   `json:"sensitive"`
}

// WorkspaceBuildParameter represents a parameter specific for a workspace build.
type WorkspaceBuildParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// WorkspaceBuild returns a single workspace build for a workspace.
// If history is "", the latest version is returned.
func (c *Client) WorkspaceBuild(ctx context.Context, id uuid.UUID) (WorkspaceBuild, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspacebuilds/%s", id), nil)
	if err != nil {
		return WorkspaceBuild{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceBuild{}, ReadBodyAsError(res)
	}
	var workspaceBuild WorkspaceBuild
	return workspaceBuild, ReadBodyAsJSON(res, &workspaceBuild)
}

// WorkspaceBuildDebugEventRequest is the request body for
// POST /api/v2/workspacebuilds/{workspacebuild}/debug-events.
type WorkspaceBuildDebugEventRequest struct {
	// ID identifies this click so a later step of the funnel can be
	// attributed to it.
	ID uuid.UUID `json:"id" format:"uuid" validate:"required"`
}

// ReportWorkspaceBuildDebugClick reports a click on the "Debug with Coder
// Agents" action of a failed workspace build for telemetry purposes. It is a
// no-op on deployments with telemetry disabled.
func (c *Client) ReportWorkspaceBuildDebugClick(ctx context.Context, buildID uuid.UUID, req WorkspaceBuildDebugEventRequest) error {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspacebuilds/%s/debug-events", buildID), req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

type CancelWorkspaceBuildStatus string

const (
	CancelWorkspaceBuildStatusRunning CancelWorkspaceBuildStatus = "running"
	CancelWorkspaceBuildStatusPending CancelWorkspaceBuildStatus = "pending"
)

type CancelWorkspaceBuildParams struct {
	// ExpectStatus ensures the build is in the expected status before canceling.
	ExpectStatus CancelWorkspaceBuildStatus `json:"expect_status,omitempty"`
}

func (c *CancelWorkspaceBuildParams) asRequestOption() RequestOption {
	return func(r *http.Request) {
		q := r.URL.Query()
		q.Set("expect_status", string(c.ExpectStatus))
		r.URL.RawQuery = q.Encode()
	}
}

// CancelWorkspaceBuild marks a workspace build job as canceled.
func (c *Client) CancelWorkspaceBuild(ctx context.Context, id uuid.UUID, req CancelWorkspaceBuildParams) error {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/workspacebuilds/%s/cancel", id), nil, req.asRequestOption())
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

// WorkspaceBuildLogsAfter streams logs for a workspace build that occurred after a specific log ID.
func (c *Client) WorkspaceBuildLogsAfter(ctx context.Context, build uuid.UUID, after int64) (<-chan ProvisionerJobLog, io.Closer, error) {
	return c.provisionerJobLogsAfter(ctx, fmt.Sprintf("/api/v2/workspacebuilds/%s/logs", build), after)
}

// WorkspaceBuildState returns the provisioner state of the build.
func (c *Client) WorkspaceBuildState(ctx context.Context, build uuid.UUID) ([]byte, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspacebuilds/%s/state", build), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	return io.ReadAll(res.Body)
}

// UpdateWorkspaceBuildStateRequest is the request body for updating the
// provisioner state of a workspace build.
type UpdateWorkspaceBuildStateRequest struct {
	// State is the provisioner state, which JSON carries as a base64 string.
	State []byte `json:"state" swaggertype:"string" format:"byte"`
}

// UpdateWorkspaceBuildState updates the provisioner state of the build without
// triggering a new build. This is useful for state-only migrations.
func (c *Client) UpdateWorkspaceBuildState(ctx context.Context, build uuid.UUID, state []byte) error {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/workspacebuilds/%s/state", build), UpdateWorkspaceBuildStateRequest{
		State: state,
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

func (c *Client) WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(ctx context.Context, username string, workspaceName string, buildNumber string) (WorkspaceBuild, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/workspace/%s/builds/%s", username, workspaceName, buildNumber), nil)
	if err != nil {
		return WorkspaceBuild{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceBuild{}, ReadBodyAsError(res)
	}
	var workspaceBuild WorkspaceBuild
	return workspaceBuild, ReadBodyAsJSON(res, &workspaceBuild)
}

func (c *Client) WorkspaceBuildParameters(ctx context.Context, build uuid.UUID) ([]WorkspaceBuildParameter, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspacebuilds/%s/parameters", build), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var params []WorkspaceBuildParameter
	return params, ReadBodyAsJSON(res, &params)
}

type TimingStage string

const (
	// Based on ProvisionerJobTimingStage
	TimingStageInit  TimingStage = "init"
	TimingStagePlan  TimingStage = "plan"
	TimingStageGraph TimingStage = "graph"
	TimingStageApply TimingStage = "apply"
	// Based on  WorkspaceAgentScriptTimingStage
	TimingStageStart TimingStage = "start"
	TimingStageStop  TimingStage = "stop"
	TimingStageCron  TimingStage = "cron"
	// Custom timing stage to represent the time taken to connect to an agent
	TimingStageConnect TimingStage = "connect"
)

type ProvisionerTiming struct {
	JobID     uuid.UUID   `json:"job_id" format:"uuid"`
	StartedAt time.Time   `json:"started_at" format:"date-time"`
	EndedAt   time.Time   `json:"ended_at" format:"date-time"`
	Stage     TimingStage `json:"stage"`
	Source    string      `json:"source"`
	Action    string      `json:"action"`
	Resource  string      `json:"resource"`
}

type AgentScriptTiming struct {
	StartedAt          time.Time                  `json:"started_at" format:"date-time"`
	EndedAt            time.Time                  `json:"ended_at" format:"date-time"`
	ExitCode           int32                      `json:"exit_code"`
	Stage              TimingStage                `json:"stage"`
	Status             WorkspaceAgentScriptStatus `json:"status"`
	DisplayName        string                     `json:"display_name"`
	WorkspaceAgentID   string                     `json:"workspace_agent_id"`
	WorkspaceAgentName string                     `json:"workspace_agent_name"`
}

type AgentConnectionTiming struct {
	StartedAt          time.Time   `json:"started_at" format:"date-time"`
	EndedAt            time.Time   `json:"ended_at" format:"date-time"`
	Stage              TimingStage `json:"stage"`
	WorkspaceAgentID   string      `json:"workspace_agent_id"`
	WorkspaceAgentName string      `json:"workspace_agent_name"`
}

type WorkspaceBuildTimings struct {
	ProvisionerTimings []ProvisionerTiming `json:"provisioner_timings"`
	// TODO: Consolidate agent-related timing metrics into a single struct when
	// updating the API version
	AgentScriptTimings     []AgentScriptTiming     `json:"agent_script_timings"`
	AgentConnectionTimings []AgentConnectionTiming `json:"agent_connection_timings"`
}

func (c *Client) WorkspaceBuildTimings(ctx context.Context, build uuid.UUID) (WorkspaceBuildTimings, error) {
	path := fmt.Sprintf("/api/v2/workspacebuilds/%s/timings", build.String())
	res, err := c.Request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return WorkspaceBuildTimings{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceBuildTimings{}, ReadBodyAsError(res)
	}
	var timings WorkspaceBuildTimings
	return timings, ReadBodyAsJSON(res, &timings)
}
