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
	ID        uuid.UUID `json:"id" validate:"required" format:"uuid"`
	Name      string    `json:"name" validate:"required"`
	CreatedAt time.Time `json:"created_at" validate:"required" format:"date-time"`
	UpdatedAt time.Time `json:"updated_at" validate:"required" format:"date-time"`
}

type OrganizationMember struct {
	UserID         uuid.UUID `db:"user_id" json:"user_id" format:"uuid"`
	OrganizationID uuid.UUID `db:"organization_id" json:"organization_id" format:"uuid"`
	CreatedAt      time.Time `db:"created_at" json:"created_at" format:"date-time"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at" format:"date-time"`
	Roles          []Role    `db:"roles" json:"roles"`
}

// CreateTemplateVersionRequest enables callers to create a new Template Version.
type CreateTemplateVersionRequest struct {
	Name    string `json:"name,omitempty" validate:"omitempty,template_version_name"`
	Message string `json:"message,omitempty" validate:"lt=1048577"`
	// TemplateID optionally associates a version with a template.
	TemplateID      uuid.UUID                `json:"template_id,omitempty" format:"uuid"`
	StorageMethod   ProvisionerStorageMethod `json:"storage_method" validate:"oneof=file,required" enums:"file"`
	FileID          uuid.UUID                `json:"file_id,omitempty" validate:"required_without=ExampleID" format:"uuid"`
	ExampleID       string                   `json:"example_id,omitempty" validate:"required_without=FileID"`
	Provisioner     ProvisionerType          `json:"provisioner" validate:"oneof=terraform echo,required"`
	ProvisionerTags map[string]string        `json:"tags"`

	UserVariableValues []VariableValue `json:"user_variable_values,omitempty"`
}

type VariableValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CreateTemplateRequest provides options when creating a template.
type CreateTemplateRequest struct {
	// Name is the name of the template.
	Name string `json:"name" validate:"template_name,required"`
	// DisplayName is the displayed name of the template.
	DisplayName string `json:"display_name,omitempty" validate:"template_display_name"`
	// Description is a description of what the template contains. It must be
	// less than 128 bytes.
	Description string `json:"description,omitempty" validate:"lt=128"`
	// Icon is a relative path or external URL that specifies
	// an icon to be displayed in the dashboard.
	Icon string `json:"icon,omitempty"`

	// VersionID is an in-progress or completed job to use as an initial version
	// of the template.
	//
	// This is required on creation to enable a user-flow of validating a
	// template works. There is no reason the data-model cannot support empty
	// templates, but it doesn't make sense for users.
	VersionID uuid.UUID `json:"template_version_id" validate:"required" format:"uuid"`

	// DefaultTTLMillis allows optionally specifying the default TTL
	// for all workspaces created from this template.
	DefaultTTLMillis *int64 `json:"default_ttl_ms,omitempty"`
	// TODO(@dean): remove max_ttl once autostop_requirement is matured
	// Only one of MaxTTLMillis or AutostopRequirement can be specified.
	MaxTTLMillis *int64 `json:"max_ttl_ms,omitempty"`
	// AutostopRequirement allows optionally specifying the autostop requirement
	// for workspaces created from this template. This is an enterprise feature.
	// Only one of MaxTTLMillis or AutostopRequirement can be specified.
	AutostopRequirement *TemplateAutostopRequirement `json:"autostop_requirement,omitempty"`
	// AutostartRequirement allows optionally specifying the autostart allowed days
	// for workspaces created from this template. This is an enterprise feature.
	AutostartRequirement *TemplateAutostartRequirement `json:"autostart_requirement,omitempty"`

	// Allow users to cancel in-progress workspace jobs.
	// *bool as the default value is "true".
	AllowUserCancelWorkspaceJobs *bool `json:"allow_user_cancel_workspace_jobs"`

	// AllowUserAutostart allows users to set a schedule for autostarting their
	// workspace. By default this is true. This can only be disabled when using
	// an enterprise license.
	AllowUserAutostart *bool `json:"allow_user_autostart"`

	// AllowUserAutostop allows users to set a custom workspace TTL to use in
	// place of the template's DefaultTTL field. By default this is true. If
	// false, the DefaultTTL will always be used. This can only be disabled when
	// using an enterprise license.
	AllowUserAutostop *bool `json:"allow_user_autostop"`

	// FailureTTLMillis allows optionally specifying the max lifetime before Coder
	// stops all resources for failed workspaces created from this template.
	FailureTTLMillis *int64 `json:"failure_ttl_ms,omitempty"`
	// TimeTilDormantMillis allows optionally specifying the max lifetime before Coder
	// locks inactive workspaces created from this template.
	TimeTilDormantMillis *int64 `json:"dormant_ttl_ms,omitempty"`
	// TimeTilDormantAutoDeleteMillis allows optionally specifying the max lifetime before Coder
	// permanently deletes dormant workspaces created from this template.
	TimeTilDormantAutoDeleteMillis *int64 `json:"delete_ttl_ms,omitempty"`

	// DisableEveryoneGroupAccess allows optionally disabling the default
	// behavior of granting the 'everyone' group access to use the template.
	// If this is set to true, the template will not be available to all users,
	// and must be explicitly granted to users or groups in the permissions settings
	// of the template.
	DisableEveryoneGroupAccess bool `json:"disable_everyone_group_access"`

	// RequireActiveVersion mandates that workspaces are built with the active
	// template version.
	RequireActiveVersion bool `json:"require_active_version"`
}

// CreateWorkspaceRequest provides options for creating a new workspace.
// Either TemplateID or TemplateVersionID must be specified. They cannot both be present.
type CreateWorkspaceRequest struct {
	// TemplateID specifies which template should be used for creating the workspace.
	TemplateID uuid.UUID `json:"template_id,omitempty" validate:"required_without=TemplateVersionID,excluded_with=TemplateVersionID" format:"uuid"`
	// TemplateVersionID can be used to specify a specific version of a template for creating the workspace.
	TemplateVersionID uuid.UUID `json:"template_version_id,omitempty" validate:"required_without=TemplateID,excluded_with=TemplateID" format:"uuid"`
	Name              string    `json:"name" validate:"workspace_name,required"`
	AutostartSchedule *string   `json:"autostart_schedule"`
	TTLMillis         *int64    `json:"ttl_ms,omitempty"`
	// RichParameterValues allows for additional parameters to be provided
	// during the initial provision.
	RichParameterValues []WorkspaceBuildParameter `json:"rich_parameter_values,omitempty"`
	AutomaticUpdates    AutomaticUpdates          `json:"automatic_updates,omitempty"`
}

func (c *Client) Organization(ctx context.Context, id uuid.UUID) (Organization, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s", id.String()), nil)
	if err != nil {
		return Organization{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Organization{}, ReadBodyAsError(res)
	}

	var organization Organization
	return organization, json.NewDecoder(res.Body).Decode(&organization)
}

// ProvisionerDaemons returns provisioner daemons available.
//
// Deprecated: We no longer track provisioner daemons as they connect.  This function may return historical data
// but new provisioner daemons will not appear.
func (c *Client) ProvisionerDaemons(ctx context.Context) ([]ProvisionerDaemon, error) {
	res, err := c.Request(ctx, http.MethodGet,
		// TODO: the organization path parameter is currently ignored.
		"/api/v2/organizations/default/provisionerdaemons",
		nil,
	)
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
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
		return TemplateVersion{}, ReadBodyAsError(res)
	}

	var templateVersion TemplateVersion
	return templateVersion, json.NewDecoder(res.Body).Decode(&templateVersion)
}

func (c *Client) TemplateVersionByOrganizationAndName(ctx context.Context, organizationID uuid.UUID, templateName, versionName string) (TemplateVersion, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/templates/%s/versions/%s", organizationID.String(), templateName, versionName),
		nil,
	)
	if err != nil {
		return TemplateVersion{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return TemplateVersion{}, ReadBodyAsError(res)
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
		return Template{}, ReadBodyAsError(res)
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
		return nil, ReadBodyAsError(res)
	}

	var templates []Template
	return templates, json.NewDecoder(res.Body).Decode(&templates)
}

// TemplateByName finds a template inside the organization provided with a case-insensitive name.
func (c *Client) TemplateByName(ctx context.Context, organizationID uuid.UUID, name string) (Template, error) {
	if name == "" {
		return Template{}, xerrors.Errorf("template name cannot be empty")
	}
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/templates/%s", organizationID.String(), name),
		nil,
	)
	if err != nil {
		return Template{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Template{}, ReadBodyAsError(res)
	}

	var template Template
	return template, json.NewDecoder(res.Body).Decode(&template)
}

// CreateWorkspace creates a new workspace for the template specified.
func (c *Client) CreateWorkspace(ctx context.Context, organizationID uuid.UUID, user string, request CreateWorkspaceRequest) (Workspace, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/members/%s/workspaces", organizationID, user), request)
	if err != nil {
		return Workspace{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return Workspace{}, ReadBodyAsError(res)
	}

	var workspace Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}
