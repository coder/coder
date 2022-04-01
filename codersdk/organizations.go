package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
)

// Organization is the JSON representation of a Coder organization.
type Organization struct {
	ID        uuid.UUID `json:"id" validate:"required"`
	Name      string    `json:"name" validate:"required"`
	CreatedAt time.Time `json:"created_at" validate:"required"`
	UpdatedAt time.Time `json:"updated_at" validate:"required"`
}

// CreateProjectVersionRequest enables callers to create a new Project Version.
type CreateProjectVersionRequest struct {
	// ProjectID optionally associates a version with a project.
	ProjectID uuid.UUID `json:"project_id"`

	StorageMethod database.ProvisionerStorageMethod `json:"storage_method" validate:"oneof=file,required"`
	StorageSource string                            `json:"storage_source" validate:"required"`
	Provisioner   database.ProvisionerType          `json:"provisioner" validate:"oneof=terraform echo,required"`
	// ParameterValues allows for additional parameters to be provided
	// during the dry-run provision stage.
	ParameterValues []CreateParameterRequest `json:"parameter_values"`
}

// CreateProjectRequest provides options when creating a project.
type CreateProjectRequest struct {
	Name string `json:"name" validate:"username,required"`

	// VersionID is an in-progress or completed job to use as
	// an initial version of the project.
	//
	// This is required on creation to enable a user-flow of validating a
	// project works. There is no reason the data-model cannot support
	// empty projects, but it doesn't make sense for users.
	VersionID       uuid.UUID                `json:"project_version_id" validate:"required"`
	ParameterValues []CreateParameterRequest `json:"parameter_values"`
}

func (c *Client) Organization(ctx context.Context, id uuid.UUID) (Organization, error) {
	res, err := c.request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s", id.String()), nil)
	if err != nil {
		return Organization{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Organization{}, readBodyAsError(res)
	}

	var organization Organization
	return organization, json.NewDecoder(res.Body).Decode(&organization)
}

// ProvisionerDaemonsByOrganization returns provisioner daemons available for an organization.
func (c *Client) ProvisionerDaemonsByOrganization(ctx context.Context, organizationID uuid.UUID) ([]ProvisionerDaemon, error) {
	res, err := c.request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/provisionerdaemons", organizationID.String()),
		nil,
	)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}

	var daemons []ProvisionerDaemon
	return daemons, json.NewDecoder(res.Body).Decode(&daemons)
}

// CreateProjectVersion processes source-code and optionally associates the version with a project.
// Executing without a project is useful for validating source-code.
func (c *Client) CreateProjectVersion(ctx context.Context, organizationID uuid.UUID, req CreateProjectVersionRequest) (ProjectVersion, error) {
	res, err := c.request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/organizations/%s/projectversions", organizationID.String()),
		req,
	)
	if err != nil {
		return ProjectVersion{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return ProjectVersion{}, readBodyAsError(res)
	}

	var projectVersion ProjectVersion
	return projectVersion, json.NewDecoder(res.Body).Decode(&projectVersion)
}

// CreateProject creates a new project inside an organization.
func (c *Client) CreateProject(ctx context.Context, organizationID uuid.UUID, request CreateProjectRequest) (Project, error) {
	res, err := c.request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/organizations/%s/projects", organizationID.String()),
		request,
	)
	if err != nil {
		return Project{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return Project{}, readBodyAsError(res)
	}

	var project Project
	return project, json.NewDecoder(res.Body).Decode(&project)
}

// ProjectsByOrganization lists all projects inside of an organization.
func (c *Client) ProjectsByOrganization(ctx context.Context, organizationID uuid.UUID) ([]Project, error) {
	res, err := c.request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/projects", organizationID.String()),
		nil,
	)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}

	var projects []Project
	return projects, json.NewDecoder(res.Body).Decode(&projects)
}

// ProjectByName finds a project inside the organization provided with a case-insensitive name.
func (c *Client) ProjectByName(ctx context.Context, organizationID uuid.UUID, name string) (Project, error) {
	res, err := c.request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/projects/%s", organizationID.String(), name),
		nil,
	)
	if err != nil {
		return Project{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Project{}, readBodyAsError(res)
	}

	var project Project
	return project, json.NewDecoder(res.Body).Decode(&project)
}
