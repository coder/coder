package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/coder/coder/coderd"
	"github.com/google/uuid"
)

// OrganizationsByUser returns organizations for the provided user.
func (c *Client) OrganizationsByUser(ctx context.Context, user uuid.UUID) ([]coderd.Organization, error) {
	return nil, nil
}

// OrganizationByName returns an organization by case-insensitive name.
func (c *Client) OrganizationByName(ctx context.Context, user uuid.UUID, name string) (coderd.Organization, error) {
	return coderd.Organization{}, nil
}

// ProvisionerDaemonsByOrganization returns provisioner daemons available for an organization.
func (c *Client) ProvisionerDaemonsByOrganization(ctx context.Context, organization string) ([]coderd.ProvisionerDaemon, error) {
	return nil, nil
}

// CreateProjectVersion processes source-code and optionally associates the version with a project.
// Executing without a project is useful for validating source-code.
func (c *Client) CreateProjectVersion(ctx context.Context, organization uuid.UUID, req coderd.CreateProjectVersion) (coderd.ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/projectversion", organization), req)
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
func (c *Client) ProjectsByOrganization(ctx context.Context, organization uuid.UUID) ([]coderd.Project, error) {
	return nil, nil
}

// ProjectByName finds a project inside the organization provided with a case-insensitive name.
func (c *Client) ProjectByName(ctx context.Context, organization uuid.UUID, name string) (coderd.Project, error) {
	return coderd.Project{}, nil
}
