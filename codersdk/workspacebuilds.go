package codersdk

import (
	"context"
	"encoding/json"
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
	// "initiator" is used when a workspace build is triggered by a user.
	// Combined with the initiator id/username, it indicates which user initiated the build.
	BuildReasonInitiator BuildReason = "initiator"
	// "autostart" is used when a build to start a workspace is triggered by Autostart.
	// The initiator id/username in this case is the workspace owner and can be ignored.
	BuildReasonAutostart BuildReason = "autostart"
	// "autostop" is used when a build to stop a workspace is triggered by Autostop.
	// The initiator id/username in this case is the workspace owner and can be ignored.
	BuildReasonAutostop BuildReason = "autostop"
)

// WorkspaceBuild is an at-point representation of a workspace state.
// BuildNumbers start at 1 and increase by 1 for each subsequent build
type WorkspaceBuild struct {
	ID                      uuid.UUID           `json:"id" format:"uuid"`
	CreatedAt               time.Time           `json:"created_at" format:"date-time"`
	UpdatedAt               time.Time           `json:"updated_at" format:"date-time"`
	WorkspaceID             uuid.UUID           `json:"workspace_id" format:"uuid"`
	WorkspaceName           string              `json:"workspace_name"`
	WorkspaceOwnerID        uuid.UUID           `json:"workspace_owner_id" format:"uuid"`
	WorkspaceOwnerName      string              `json:"workspace_owner_name"`
	WorkspaceOwnerAvatarURL string              `json:"workspace_owner_avatar_url"`
	TemplateVersionID       uuid.UUID           `json:"template_version_id" format:"uuid"`
	TemplateVersionName     string              `json:"template_version_name"`
	BuildNumber             int32               `json:"build_number"`
	Transition              WorkspaceTransition `json:"transition" enums:"start,stop,delete"`
	InitiatorID             uuid.UUID           `json:"initiator_id" format:"uuid"`
	InitiatorUsername       string              `json:"initiator_name"`
	Job                     ProvisionerJob      `json:"job"`
	Reason                  BuildReason         `db:"reason" json:"reason" enums:"initiator,autostart,autostop"`
	Resources               []WorkspaceResource `json:"resources"`
	Deadline                NullTime            `json:"deadline,omitempty" format:"date-time"`
	MaxDeadline             NullTime            `json:"max_deadline,omitempty" format:"date-time"`
	Status                  WorkspaceStatus     `json:"status" enums:"pending,starting,running,stopping,stopped,failed,canceling,canceled,deleting,deleted"`
	DailyCost               int32               `json:"daily_cost"`
}

// WorkspaceResource describes resources used to create a workspace, for instance:
// containers, images, volumes.
type WorkspaceResource struct {
	ID         uuid.UUID                   `json:"id" format:"uuid"`
	CreatedAt  time.Time                   `json:"created_at" format:"date-time"`
	JobID      uuid.UUID                   `json:"job_id" format:"uuid"`
	Transition WorkspaceTransition         `json:"workspace_transition" enums:"start,stop,delete"`
	Type       string                      `json:"type"`
	Name       string                      `json:"name"`
	Hide       bool                        `json:"hide"`
	Icon       string                      `json:"icon"`
	Agents     []WorkspaceAgent            `json:"agents,omitempty"`
	Metadata   []WorkspaceResourceMetadata `json:"metadata,omitempty"`
	DailyCost  int32                       `json:"daily_cost"`
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
	return workspaceBuild, json.NewDecoder(res.Body).Decode(&workspaceBuild)
}

// CancelWorkspaceBuild marks a workspace build job as canceled.
func (c *Client) CancelWorkspaceBuild(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/workspacebuilds/%s/cancel", id), nil)
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
	return workspaceBuild, json.NewDecoder(res.Body).Decode(&workspaceBuild)
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
	return params, json.NewDecoder(res.Body).Decode(&params)
}
