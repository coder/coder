package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd"
)

// CreateProject creates a new project inside an organization.
func (c *Client) CreateProject(ctx context.Context, organization uuid.UUID, request coderd.CreateProjectRequest) (coderd.Project, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/projects", organization), request)
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

// ProjectsByOrganization lists all projects inside of an organization.
func (c *Client) ProjectsByOrganization(ctx context.Context, organization uuid.UUID) ([]coderd.Project, error) {
	return nil, nil
}

// ProjectByName finds a project inside the organization provided with a case-insensitive name.
func (c *Client) ProjectByName(ctx context.Context, organization uuid.UUID, name string) (coderd.Project, error) {
	return coderd.Project{}, nil
}

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
