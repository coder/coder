package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd"
)

// Project returns a single project.
func (c *Client) Project(ctx context.Context, project uuid.UUID) (coderd.Project, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/project/%s", project), nil)
	if err != nil {
		return coderd.Project{}, nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.Project{}, readBodyAsError(res)
	}
	var resp coderd.Project
	return resp, json.NewDecoder(res.Body).Decode(&resp)
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

// ProjectParameters returns parameters scoped to a project.
func (c *Client) ProjectParameters(ctx context.Context, project uuid.UUID) ([]coderd.ParameterValue, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/project/%s/parameters", project), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var params []coderd.ParameterValue
	return params, json.NewDecoder(res.Body).Decode(&params)
}

// CreateProjectParameter creates a new parameter value scoped to a project.
func (c *Client) CreateProjectParameter(ctx context.Context, organization, project string, req coderd.CreateParameterValueRequest) (coderd.ParameterValue, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/projects/%s/%s/parameters", organization, project), req)
	if err != nil {
		return coderd.ParameterValue{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.ParameterValue{}, readBodyAsError(res)
	}
	var param coderd.ParameterValue
	return param, json.NewDecoder(res.Body).Decode(&param)
}

// ProjectVersionsByProject lists versions associated with a project.
func (c *Client) ProjectVersionsByProject(ctx context.Context, project uuid.UUID) ([]coderd.ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/project/%s/versions", project), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var projectVersion []coderd.ProjectVersion
	return projectVersion, json.NewDecoder(res.Body).Decode(&projectVersion)
}

func (c *Client) DeleteProject(ctx context.Context, project uuid.UUID) error {
	return nil
}

func (c *Client) UpdateProjectVersion() {

}
