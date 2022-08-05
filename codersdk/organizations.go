package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type ProvisionerStorageMethod string

const (
	ProvisionerStorageMethodFile ProvisionerStorageMethod = "file"
)

type ProvisionerType string

const (
	ProvisionerTypeEcho      ProvisionerType = "echo"
	ProvisionerTypeTerraform ProvisionerType = "terraform"
)

// Organization is the JSON representation of a Coder organization.
type Organization struct {
	ID        uuid.UUID `json:"id" validate:"required"`
	Name      string    `json:"name" validate:"required"`
	CreatedAt time.Time `json:"created_at" validate:"required"`
	UpdatedAt time.Time `json:"updated_at" validate:"required"`
}

// CreateTemplateVersionRequest enables callers to create a new Template Version.
type CreateTemplateVersionRequest struct {
	// TemplateID optionally associates a version with a template.
	TemplateID uuid.UUID `json:"template_id,omitempty"`

	StorageMethod ProvisionerStorageMethod `json:"storage_method" validate:"oneof=file,required"`
	StorageSource string                   `json:"storage_source" validate:"required"`
	Provisioner   ProvisionerType          `json:"provisioner" validate:"oneof=terraform echo,required"`
	// ParameterValues allows for additional parameters to be provided
	// during the dry-run provision stage.
	ParameterValues []CreateParameterRequest `json:"parameter_values,omitempty"`
}

// CreateTemplateRequest provides options when creating a template.
type CreateTemplateRequest struct {
	// Name is the name of the template.
	Name string `json:"name" validate:"template_name,required"`
	// Description is a description of what the template contains. It must be
	// less than 128 bytes.
	Description string `json:"description,omitempty" validate:"lt=128"`

	// VersionID is an in-progress or completed job to use as an initial version
	// of the template.
	//
	// This is required on creation to enable a user-flow of validating a
	// template works. There is no reason the data-model cannot support empty
	// templates, but it doesn't make sense for users.
	VersionID       uuid.UUID                `json:"template_version_id" validate:"required"`
	ParameterValues []CreateParameterRequest `json:"parameter_values,omitempty"`

	// MaxTTLMillis allows optionally specifying the maximum allowable TTL
	// for all workspaces created from this template.
	MaxTTLMillis *int64 `json:"max_ttl_ms,omitempty"`

	// MinAutostartIntervalMillis allows optionally specifying the minimum
	// allowable duration between autostarts for all workspaces created from
	// this template.
	MinAutostartIntervalMillis *int64 `json:"min_autostart_interval_ms,omitempty"`
}

// CreateWorkspaceRequest provides options for creating a new workspace.
type CreateWorkspaceRequest struct {
	TemplateID        uuid.UUID `json:"template_id" validate:"required"`
	Name              string    `json:"name" validate:"workspace_name,required"`
	AutostartSchedule *string   `json:"autostart_schedule"`
	TTLMillis         *int64    `json:"ttl_ms,omitempty"`
	// ParameterValues allows for additional parameters to be provided
	// during the initial provision.
	ParameterValues []CreateParameterRequest `json:"parameter_values,omitempty"`
}

func (c *Client) Organization(ctx context.Context, id uuid.UUID) (Organization, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s", id.String()), nil)
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
func (c *Client) ProvisionerDaemons(ctx context.Context) ([]ProvisionerDaemon, error) {
	res, err := c.Request(ctx, http.MethodGet,
		"/api/v2/provisionerdaemons",
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

// CreateTemplateVersion processes source-code and optionally associates the version with a template.
// Executing without a template is useful for validating source-code.
func (c *Client) CreateTemplateVersion(ctx context.Context, organizationID uuid.UUID, req CreateTemplateVersionRequest) (TemplateVersion, error) {
	res, err := c.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/organizations/%s/templateversions", organizationID.String()),
		req,
	)
	if err != nil {
		return TemplateVersion{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return TemplateVersion{}, readBodyAsError(res)
	}

	var templateVersion TemplateVersion
	return templateVersion, json.NewDecoder(res.Body).Decode(&templateVersion)
}

// CreateTemplate creates a new template inside an organization.
func (c *Client) CreateTemplate(ctx context.Context, organizationID uuid.UUID, request CreateTemplateRequest) (Template, error) {
	res, err := c.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/organizations/%s/templates", organizationID.String()),
		request,
	)
	if err != nil {
		return Template{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return Template{}, readBodyAsError(res)
	}

	var template Template
	return template, json.NewDecoder(res.Body).Decode(&template)
}

// TemplatesByOrganization lists all templates inside of an organization.
func (c *Client) TemplatesByOrganization(ctx context.Context, organizationID uuid.UUID) ([]Template, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/templates", organizationID.String()),
		nil,
	)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}

	var templates []Template
	return templates, json.NewDecoder(res.Body).Decode(&templates)
}

// TemplateByName finds a template inside the organization provided with a case-insensitive name.
func (c *Client) TemplateByName(ctx context.Context, organizationID uuid.UUID, name string) (Template, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/templates/%s", organizationID.String(), name),
		nil,
	)
	if err != nil {
		return Template{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Template{}, readBodyAsError(res)
	}

	var template Template
	return template, json.NewDecoder(res.Body).Decode(&template)
}

// CreateWorkspace creates a new workspace for the template specified.
func (c *Client) CreateWorkspace(ctx context.Context, organizationID uuid.UUID, request CreateWorkspaceRequest) (Workspace, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/workspaces", organizationID), request)
	if err != nil {
		return Workspace{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return Workspace{}, readBodyAsError(res)
	}

	var workspace Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}
