package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/database"
)

// WorkspaceBuild is an at-point representation of a workspace state.
// Iterate on before/after to determine a chronological history.
type WorkspaceBuild struct {
	ID               uuid.UUID                    `json:"id"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
	WorkspaceID      uuid.UUID                    `json:"workspace_id"`
	ProjectVersionID uuid.UUID                    `json:"project_version_id"`
	BeforeID         uuid.UUID                    `json:"before_id"`
	AfterID          uuid.UUID                    `json:"after_id"`
	Name             string                       `json:"name"`
	Transition       database.WorkspaceTransition `json:"transition"`
	Initiator        string                       `json:"initiator"`
	Job              ProvisionerJob               `json:"job"`
}

// WorkspaceBuild returns a single workspace build for a workspace.
// If history is "", the latest version is returned.
func (c *Client) WorkspaceBuild(ctx context.Context, id uuid.UUID) (WorkspaceBuild, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspacebuilds/%s", id), nil)
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

// WorkspaceResourcesByBuild returns resources for a workspace build.
func (c *Client) WorkspaceResourcesByBuild(ctx context.Context, build uuid.UUID) ([]WorkspaceResource, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspacebuilds/%s/resources", build), nil)
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
func (c *Client) WorkspaceBuildLogsBefore(ctx context.Context, version uuid.UUID, before time.Time) ([]ProvisionerJobLog, error) {
	return c.provisionerJobLogsBefore(ctx, fmt.Sprintf("/api/v2/workspacebuilds/%s/logs", version), before)
}

// WorkspaceBuildLogsAfter streams logs for a workspace build that occurred after a specific time.
func (c *Client) WorkspaceBuildLogsAfter(ctx context.Context, version uuid.UUID, after time.Time) (<-chan ProvisionerJobLog, error) {
	return c.provisionerJobLogsAfter(ctx, fmt.Sprintf("/api/v2/workspacebuilds/%s/logs", version), after)
}
