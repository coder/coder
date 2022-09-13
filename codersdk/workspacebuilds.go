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
	ID                 uuid.UUID           `json:"id"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
	WorkspaceID        uuid.UUID           `json:"workspace_id"`
	WorkspaceName      string              `json:"workspace_name"`
	WorkspaceOwnerID   uuid.UUID           `json:"workspace_owner_id"`
	WorkspaceOwnerName string              `json:"workspace_owner_name"`
	TemplateVersionID  uuid.UUID           `json:"template_version_id"`
	BuildNumber        int32               `json:"build_number"`
	Transition         WorkspaceTransition `json:"transition"`
	InitiatorID        uuid.UUID           `json:"initiator_id"`
	InitiatorUsername  string              `json:"initiator_name"`
	Job                ProvisionerJob      `json:"job"`
	Deadline           NullTime            `json:"deadline,omitempty"`
	Reason             BuildReason         `db:"reason" json:"reason"`
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
		return WorkspaceBuild{}, readBodyAsError(res)
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
		return readBodyAsError(res)
	}
	return nil
}

// WorkspaceResourcesByBuild returns resources for a workspace build.
func (c *Client) WorkspaceResourcesByBuild(ctx context.Context, build uuid.UUID) ([]WorkspaceResource, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspacebuilds/%s/resources", build), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var resources []WorkspaceResource
	return resources, json.NewDecoder(res.Body).Decode(&resources)
}

// WorkspaceBuildLogsBefore returns logs that occurred before a specific time.
func (c *Client) WorkspaceBuildLogsBefore(ctx context.Context, build uuid.UUID, before time.Time) ([]ProvisionerJobLog, error) {
	return c.provisionerJobLogsBefore(ctx, fmt.Sprintf("/api/v2/workspacebuilds/%s/logs", build), before)
}

// WorkspaceBuildLogsAfter streams logs for a workspace build that occurred after a specific time.
func (c *Client) WorkspaceBuildLogsAfter(ctx context.Context, build uuid.UUID, after time.Time) (<-chan ProvisionerJobLog, error) {
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
		return nil, readBodyAsError(res)
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
		return WorkspaceBuild{}, readBodyAsError(res)
	}
	var workspaceBuild WorkspaceBuild
	return workspaceBuild, json.NewDecoder(res.Body).Decode(&workspaceBuild)
}
