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

// Workspaces returns all workspaces the authenticated session has access to.
func (c *Client) WorkspacesByUser(ctx context.Context, user uuid.UUID) ([]coderd.Workspace, error) {
	route := fmt.Sprintf("/api/v2/user/%s/workspaces", user)
	if user == Me {
		route = fmt.Sprintf("/api/v2/user/me/workspaces", user)
	}
	res, err := c.request(ctx, http.MethodGet, route, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var workspaces []coderd.Workspace
	return workspaces, json.NewDecoder(res.Body).Decode(&workspaces)
}

// WorkspacesByProject lists all workspaces for a specific project.
func (c *Client) WorkspacesByProject(ctx context.Context, project uuid.UUID) ([]coderd.Workspace, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s/workspaces", project), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var workspaces []coderd.Workspace
	return workspaces, json.NewDecoder(res.Body).Decode(&workspaces)
}

// WorkspaceByName returns a workspace for a user that matches the case-insensitive name.
func (c *Client) WorkspaceByName(ctx context.Context, user uuid.UUID, name string) (coderd.Workspace, error) {
	return coderd.Workspace{}, nil
}

// Workspace returns a single workspace by owner and name.
func (c *Client) Workspace(ctx context.Context, id uuid.UUID) (coderd.Workspace, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspace/%s", id), nil)
	if err != nil {
		return coderd.Workspace{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.Workspace{}, readBodyAsError(res)
	}
	var workspace coderd.Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}

// WorkspaceProvisions returns a historical list of provision operations for a workspace.
func (c *Client) WorkspaceProvisions(ctx context.Context, workspace uuid.UUID) ([]coderd.WorkspaceBuild, error) {
	return nil, nil
}

// WorkspaceProvision returns
func (c *Client) WorkspaceVersion(ctx context.Context, provision uuid.UUID) (coderd.WorkspaceBuild, error) {
	return coderd.WorkspaceBuild{}, nil
}

// ListWorkspaceBuild returns historical data for workspace builds.
func (c *Client) ListWorkspaceBuild(ctx context.Context, owner, workspace string) ([]coderd.WorkspaceBuild, error) {
	if owner == "" {
		owner = "me"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/%s/version", owner, workspace), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var workspaceBuild []coderd.WorkspaceBuild
	return workspaceBuild, json.NewDecoder(res.Body).Decode(&workspaceBuild)
}

// WorkspaceBuild returns a single workspace build for a workspace.
// If history is "", the latest version is returned.
func (c *Client) WorkspaceBuild(ctx context.Context, owner, workspace, history string) (coderd.WorkspaceBuild, error) {
	if owner == "" {
		owner = "me"
	}
	if history == "" {
		history = "latest"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/%s/version/%s", owner, workspace, history), nil)
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

// CreateWorkspace creates a new workspace for the project specified.
func (c *Client) CreateWorkspace(ctx context.Context, user string, request coderd.CreateWorkspaceRequest) (coderd.Workspace, error) {
	if user == "" {
		user = "me"
	}
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspaces/%s", user), request)
	if err != nil {
		return coderd.Workspace{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.Workspace{}, readBodyAsError(res)
	}
	var workspace coderd.Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}

// CreateWorkspaceBuild queues a new build to occur for a workspace.
func (c *Client) CreateWorkspaceBuild(ctx context.Context, owner, workspace string, request coderd.CreateWorkspaceBuildRequest) (coderd.WorkspaceBuild, error) {
	if owner == "" {
		owner = "me"
	}
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspaces/%s/%s/version", owner, workspace), request)
	if err != nil {
		return coderd.WorkspaceBuild{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.WorkspaceBuild{}, readBodyAsError(res)
	}
	var workspaceBuild coderd.WorkspaceBuild
	return workspaceBuild, json.NewDecoder(res.Body).Decode(&workspaceBuild)
}

func (c *Client) WorkspaceProvisionJob(ctx context.Context, organization string, job uuid.UUID) (coderd.ProvisionerJob, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceprovision/%s/%s", organization, job), nil)
	if err != nil {
		return coderd.ProvisionerJob{}, nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.ProvisionerJob{}, readBodyAsError(res)
	}
	var resp coderd.ProvisionerJob
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// WorkspaceProvisionJobLogsBefore returns logs that occurred before a specific time.
func (c *Client) WorkspaceProvisionJobLogsBefore(ctx context.Context, organization string, job uuid.UUID, before time.Time) ([]coderd.ProvisionerJobLog, error) {
	return c.provisionerJobLogsBefore(ctx, "workspaceprovision", organization, job, before)
}

// WorkspaceProvisionJobLogsAfter streams logs for a workspace provision operation that occurred after a specific time.
func (c *Client) WorkspaceProvisionJobLogsAfter(ctx context.Context, organization string, job uuid.UUID, after time.Time) (<-chan coderd.ProvisionerJobLog, error) {
	return c.provisionerJobLogsAfter(ctx, "workspaceprovision", organization, job, after)
}

func (c *Client) WorkspaceProvisionJobResources(ctx context.Context, organization string, job uuid.UUID) ([]coderd.ProvisionerJobResource, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaceprovision/%s/%s/resources", organization, job), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var resources []coderd.ProvisionerJobResource
	return resources, json.NewDecoder(res.Body).Decode(&resources)
}
