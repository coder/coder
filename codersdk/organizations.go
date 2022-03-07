package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd"
)

func (c *Client) Organization(ctx context.Context, id string) (coderd.Organization, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s", id), nil)
	if err != nil {
		return coderd.Organization{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.Organization{}, readBodyAsError(res)
	}
	var organization coderd.Organization
	return organization, json.NewDecoder(res.Body).Decode(&organization)
}

// ProvisionerDaemonsByOrganization returns provisioner daemons available for an organization.
func (c *Client) ProvisionerDaemonsByOrganization(ctx context.Context, organization string) ([]coderd.ProvisionerDaemon, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/provisionerdaemons", organization), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	var daemons []coderd.ProvisionerDaemon
	return daemons, json.NewDecoder(res.Body).Decode(&daemons)
}

// CreateProjectVersion processes source-code and optionally associates the version with a project.
// Executing without a project is useful for validating source-code.
func (c *Client) CreateProjectVersion(ctx context.Context, organization string, req coderd.CreateProjectVersionRequest) (coderd.ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/projectversions", organization), req)
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

// CreateProject creates a new project inside an organization.
func (c *Client) CreateProject(ctx context.Context, organization string, request coderd.CreateProjectRequest) (coderd.Project, error) {
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
func (c *Client) ProjectsByOrganization(ctx context.Context, organization string) ([]coderd.Project, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/projects", organization), nil)
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

// ProjectByName finds a project inside the organization provided with a case-insensitive name.
func (c *Client) ProjectByName(ctx context.Context, organization, name string) (coderd.Project, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/projects/%s", organization, name), nil)
	if err != nil {
		return coderd.Project{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return coderd.Project{}, readBodyAsError(res)
	}
	var project coderd.Project
	return project, json.NewDecoder(res.Body).Decode(&project)
}
