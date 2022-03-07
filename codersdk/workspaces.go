package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd"
)

// Workspace returns a single workspace.
func (c *Client) Workspace(ctx context.Context, id uuid.UUID) (coderd.Workspace, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s", id), nil)
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

func (c *Client) WorkspaceBuilds(ctx context.Context, workspace uuid.UUID) ([]coderd.WorkspaceBuild, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/builds", workspace), nil)
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

// CreateWorkspaceBuild queues a new build to occur for a workspace.
func (c *Client) CreateWorkspaceBuild(ctx context.Context, workspace uuid.UUID, request coderd.CreateWorkspaceBuildRequest) (coderd.WorkspaceBuild, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspaces/%s/builds", workspace), request)
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

func (c *Client) WorkspaceBuildByName(ctx context.Context, workspace uuid.UUID, name string) (coderd.WorkspaceBuild, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/builds/%s", workspace, name), nil)
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

func (c *Client) WorkspaceBuildLatest(ctx context.Context, workspace uuid.UUID) (coderd.WorkspaceBuild, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/builds/latest", workspace), nil)
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
