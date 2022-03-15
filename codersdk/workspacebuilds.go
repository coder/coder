package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd"
)

// WorkspaceBuild returns a single workspace build for a workspace.
// If history is "", the latest version is returned.
func (c *Client) WorkspaceBuild(ctx context.Context, id uuid.UUID) (coderd.WorkspaceBuild, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspacebuilds/%s", id), nil)
	if err != nil {
		return coderd.WorkspaceBuild{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.WorkspaceBuild{}, readBodyAsError(res)
	}
	var workspaceBuild coderd.WorkspaceBuild
	return workspaceBuild, json.NewDecoder(res.Body).Decode(&workspaceBuild)
}

// WorkspaceResourcesByBuild returns resources for a workspace build.
func (c *Client) WorkspaceResourcesByBuild(ctx context.Context, build uuid.UUID) ([]coderd.WorkspaceResource, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspacebuilds/%s/resources", build), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var resources []coderd.WorkspaceResource
	return resources, json.NewDecoder(res.Body).Decode(&resources)
}

// WorkspaceBuildLogsBefore returns logs that occurred before a specific time.
func (c *Client) WorkspaceBuildLogsBefore(ctx context.Context, version uuid.UUID, before time.Time) ([]coderd.ProvisionerJobLog, error) {
	return c.provisionerJobLogsBefore(ctx, fmt.Sprintf("/api/v2/workspacebuilds/%s/logs", version), before)
}

// WorkspaceBuildLogsAfter streams logs for a workspace build that occurred after a specific time.
func (c *Client) WorkspaceBuildLogsAfter(ctx context.Context, version uuid.UUID, after time.Time) (<-chan coderd.ProvisionerJobLog, error) {
	return c.provisionerJobLogsAfter(ctx, fmt.Sprintf("/api/v2/workspacebuilds/%s/logs", version), after)
}
