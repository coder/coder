package codersdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd"
)

// Workspace returns a single workspace by owner and name.
func (c *Client) Workspace(ctx context.Context, owner, name string) (coderd.Workspace, error) {
	if owner == "" {
		owner = "me"
	}
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspace/%s/%s", owner, name), nil)
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

// Workspaces returns all workspaces for an owner.
// If owner is empty, all workspaces the caller has access to will be returned.
func (c *Client) Workspaces(ctx context.Context, owner, project string) ([]coderd.Workspace, error) {
	route := "/api/v2/workspaces"
	if owner != "" {
		route += fmt.Sprintf("/%s", owner)
	}
	if project != "" {
		if owner == "" {
			return nil, errors.New("owner must not be empty if project is provided")
		}
		route += fmt.Sprintf("/%s", project)
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

func (c *Client) CreateWorkspace(ctx context.Context, owner, project string, request coderd.CreateWorkspaceRequest) (coderd.Workspace, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspaces/%s/%s", owner, project), request)
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
