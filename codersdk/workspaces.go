package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd"
)

// Workspaces returns all workspaces the authenticated session has access to.
// If owner is specified, all workspaces for an organization will be returned.
// If owner is empty, all workspaces the caller has access to will be returned.
func (c *Client) WorkspacesByUser(ctx context.Context, user string) ([]coderd.Workspace, error) {
	route := "/api/v2/workspaces"
	if user != "" {
		route += fmt.Sprintf("/%s", user)
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
func (c *Client) WorkspacesByProject(ctx context.Context, organization, project string) ([]coderd.Workspace, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s/%s/workspaces", organization, project), nil)
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

// Workspace returns a single workspace by owner and name.
func (c *Client) Workspace(ctx context.Context, owner, name string) (coderd.Workspace, error) {
	if owner == "" {
		owner = "me"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/%s", owner, name), nil)
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

// WorkspaceHistory returns historical data for workspace builds.
func (c *Client) WorkspaceHistory(ctx context.Context, owner, workspace string) ([]coderd.WorkspaceHistory, error) {
	if owner == "" {
		owner = "me"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/%s/history", owner, workspace), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var workspaceHistory []coderd.WorkspaceHistory
	return workspaceHistory, json.NewDecoder(res.Body).Decode(&workspaceHistory)
}

// LatestWorkspaceHistory returns the newest build for a workspace.
func (c *Client) LatestWorkspaceHistory(ctx context.Context, owner, workspace string) (coderd.WorkspaceHistory, error) {
	if owner == "" {
		owner = "me"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/%s/history/latest", owner, workspace), nil)
	if err != nil {
		return coderd.WorkspaceHistory{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.WorkspaceHistory{}, readBodyAsError(res)
	}
	var workspaceHistory coderd.WorkspaceHistory
	return workspaceHistory, json.NewDecoder(res.Body).Decode(&workspaceHistory)
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

// CreateWorkspaceHistory queues a new build to occur for a workspace.
func (c *Client) CreateWorkspaceHistory(ctx context.Context, owner, workspace string, request coderd.CreateWorkspaceHistoryRequest) (coderd.WorkspaceHistory, error) {
	if owner == "" {
		owner = "me"
	}
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspaces/%s/%s/history", owner, workspace), request)
	if err != nil {
		return coderd.WorkspaceHistory{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.WorkspaceHistory{}, readBodyAsError(res)
	}
	var workspaceHistory coderd.WorkspaceHistory
	return workspaceHistory, json.NewDecoder(res.Body).Decode(&workspaceHistory)
}
