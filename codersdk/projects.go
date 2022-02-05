package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd"
)

// Projects lists projects inside an organization.
// If organization is an empty string, all projects will be returned
// for the authenticated user.
func (c *Client) Projects(ctx context.Context, organization string) ([]coderd.Project, error) {
	route := "/api/v2/projects"
	if organization != "" {
		route = fmt.Sprintf("/api/v2/projects/%s", organization)
	}
	res, err := c.request(ctx, http.MethodGet, route, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var projects []coderd.Project
	return projects, json.NewDecoder(res.Body).Decode(&projects)
}

// Project returns a single project.
func (c *Client) Project(ctx context.Context, organization, project string) (coderd.Project, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s/%s", organization, project), nil)
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

// CreateProject creates a new project inside an organization.
func (c *Client) CreateProject(ctx context.Context, organization string, request coderd.CreateProjectRequest) (coderd.Project, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/projects/%s", organization), request)
	if err != nil {
		return coderd.Project{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.Project{}, readBodyAsError(res)
	}
	var project coderd.Project
	return project, json.NewDecoder(res.Body).Decode(&project)
}

// ProjectVersions lists versions of a project.
func (c *Client) ProjectVersions(ctx context.Context, organization, project string) ([]coderd.ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s/%s/versions", organization, project), nil)
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

// ProjectVersion returns project version by name.
func (c *Client) ProjectVersion(ctx context.Context, organization, project, version string) (coderd.ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s/%s/versions/%s", organization, project, version), nil)
	if err != nil {
		return coderd.ProjectVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.ProjectVersion{}, readBodyAsError(res)
	}
	var projectVersion coderd.ProjectVersion
	return projectVersion, json.NewDecoder(res.Body).Decode(&projectVersion)
}

// CreateProjectVersion inserts a new version for the project.
func (c *Client) CreateProjectVersion(ctx context.Context, organization, project string, request coderd.CreateProjectVersionRequest) (coderd.ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/projects/%s/%s/versions", organization, project), request)
	if err != nil {
		return coderd.ProjectVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return coderd.ProjectVersion{}, readBodyAsError(res)
	}
	var projectVersion coderd.ProjectVersion
	return projectVersion, json.NewDecoder(res.Body).Decode(&projectVersion)
}

// ProjectVersionParameters returns project parameters for a version by name.
func (c *Client) ProjectVersionParameters(ctx context.Context, organization, project, version string) ([]coderd.ProjectParameter, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s/%s/versions/%s/parameters", organization, project, version), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var params []coderd.ProjectParameter
	return params, json.NewDecoder(res.Body).Decode(&params)
}

// ProjectParameters returns parameters scoped to a project.
func (c *Client) ProjectParameters(ctx context.Context, organization, project string) ([]coderd.ParameterValue, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/projects/%s/%s/parameters", organization, project), nil)
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
