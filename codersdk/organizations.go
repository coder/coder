package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// DefaultOrganization is used as a replacement for the default organization.
var DefaultOrganization = "default"

type ProvisionerStorageMethod string

const (
	ProvisionerStorageMethodFile ProvisionerStorageMethod = "file"
)

type ProvisionerType string

const (
	ProvisionerTypeEcho      ProvisionerType = "echo"
	ProvisionerTypeTerraform ProvisionerType = "terraform"
)

// ProvisionerTypeValid accepts string or ProvisionerType for easier usage.
// Will validate the enum is in the set.
func ProvisionerTypeValid[T ProvisionerType | string](pt T) error {
	switch string(pt) {
	case string(ProvisionerTypeEcho), string(ProvisionerTypeTerraform):
		return nil
	default:
		return xerrors.Errorf("provisioner type '%s' is not supported", pt)
	}
}

type MinimalOrganization struct {
	ID          uuid.UUID `table:"id" json:"id" validate:"required" format:"uuid"`
	Name        string    `table:"name,default_sort" json:"name"`
	DisplayName string    `table:"display name" json:"display_name"`
	Icon        string    `table:"icon" json:"icon"`
}

// Organization is the JSON representation of a Coder organization.
type Organization struct {
	MinimalOrganization `table:"m,recursive_inline"`
	Description         string    `table:"description" json:"description"`
	CreatedAt           time.Time `table:"created at" json:"created_at" validate:"required" format:"date-time"`
	UpdatedAt           time.Time `table:"updated at" json:"updated_at" validate:"required" format:"date-time"`
	IsDefault           bool      `table:"default" json:"is_default" validate:"required"`
}

func (o Organization) HumanName() string {
	if o.DisplayName == "" {
		return o.Name
	}
	return o.DisplayName
}

type OrganizationMember struct {
	UserID         uuid.UUID  `table:"user id" json:"user_id" format:"uuid"`
	OrganizationID uuid.UUID  `table:"organization id" json:"organization_id" format:"uuid"`
	CreatedAt      time.Time  `table:"created at" json:"created_at" format:"date-time"`
	UpdatedAt      time.Time  `table:"updated at" json:"updated_at" format:"date-time"`
	Roles          []SlimRole `table:"organization roles" json:"roles"`
}

type OrganizationMemberWithUserData struct {
	Username           string     `table:"username,default_sort" json:"username"`
	Name               string     `table:"name" json:"name"`
	AvatarURL          string     `json:"avatar_url"`
	Email              string     `json:"email"`
	GlobalRoles        []SlimRole `json:"global_roles"`
	OrganizationMember `table:"m,recursive_inline"`
}

type CreateOrganizationRequest struct {
	Name string `json:"name" validate:"required,organization_name"`
	// DisplayName will default to the same value as `Name` if not provided.
	DisplayName string `json:"display_name,omitempty" validate:"omitempty,organization_display_name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

type UpdateOrganizationRequest struct {
	Name        string  `json:"name,omitempty" validate:"omitempty,organization_name"`
	DisplayName string  `json:"display_name,omitempty" validate:"omitempty,organization_display_name"`
	Description *string `json:"description,omitempty"`
	Icon        *string `json:"icon,omitempty"`
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
	// ActivityBumpMillis allows optionally specifying the activity bump
	// duration for all workspaces created from this template. Defaults to 1h
	// but can be set to 0 to disable activity bumping.
	ActivityBumpMillis *int64 `json:"activity_bump_ms,omitempty"`
	// AutostopRequirement allows optionally specifying the autostop requirement
	// for workspaces created from this template. This is an enterprise feature.
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
	AllowUserAutostart *bool `json:"allow_user_autostart,omitempty"`

	// AllowUserAutostop allows users to set a custom workspace TTL to use in
	// place of the template's DefaultTTL field. By default this is true. If
	// false, the DefaultTTL will always be used. This can only be disabled when
	// using an enterprise license.
	AllowUserAutostop *bool `json:"allow_user_autostop,omitempty"`

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

	// MaxPortShareLevel allows optionally specifying the maximum port share level
	// for workspaces created from the template.
	MaxPortShareLevel *WorkspaceAgentPortShareLevel `json:"max_port_share_level"`
}

// CreateWorkspaceRequest provides options for creating a new workspace.
// Either TemplateID or TemplateVersionID must be specified. They cannot both be present.
// @Description CreateWorkspaceRequest provides options for creating a new workspace.
// @Description Only one of TemplateID or TemplateVersionID can be specified, not both.
// @Description If TemplateID is specified, the active version of the template will be used.
type CreateWorkspaceRequest struct {
	// TemplateID specifies which template should be used for creating the workspace.
	TemplateID uuid.UUID `json:"template_id,omitempty" validate:"required_without=TemplateVersionID,excluded_with=TemplateVersionID" format:"uuid"`
	// TemplateVersionID can be used to specify a specific version of a template for creating the workspace.
	TemplateVersionID uuid.UUID `json:"template_version_id,omitempty" validate:"required_without=TemplateID,excluded_with=TemplateID" format:"uuid"`
	Name              string    `json:"name" validate:"workspace_name,required"`
	AutostartSchedule *string   `json:"autostart_schedule,omitempty"`
	TTLMillis         *int64    `json:"ttl_ms,omitempty"`
	// RichParameterValues allows for additional parameters to be provided
	// during the initial provision.
	RichParameterValues      []WorkspaceBuildParameter `json:"rich_parameter_values,omitempty"`
	AutomaticUpdates         AutomaticUpdates          `json:"automatic_updates,omitempty"`
	TemplateVersionPresetID  uuid.UUID                 `json:"template_version_preset_id,omitempty" format:"uuid"`
	ClaimPrebuildIfAvailable bool                      `json:"claim_prebuild_if_available,omitempty"`
}

func (c *Client) OrganizationByName(ctx context.Context, name string) (Organization, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s", name), nil)
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

func (c *Client) Organizations(ctx context.Context) ([]Organization, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/organizations", nil)
	if err != nil {
		return []Organization{}, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return []Organization{}, ReadBodyAsError(res)
	}

	var organizations []Organization
	return organizations, json.NewDecoder(res.Body).Decode(&organizations)
}

func (c *Client) Organization(ctx context.Context, id uuid.UUID) (Organization, error) {
	// OrganizationByName uses the exact same endpoint. It accepts a name or uuid.
	// We just provide this function for type safety.
	return c.OrganizationByName(ctx, id.String())
}

// CreateOrganization creates an organization and adds the user making the request as an owner.
func (c *Client) CreateOrganization(ctx context.Context, req CreateOrganizationRequest) (Organization, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/organizations", req)
	if err != nil {
		return Organization{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		return Organization{}, ReadBodyAsError(res)
	}

	var org Organization
	return org, json.NewDecoder(res.Body).Decode(&org)
}

// UpdateOrganization will update information about the corresponding organization, based on
// the UUID/name provided as `orgID`.
func (c *Client) UpdateOrganization(ctx context.Context, orgID string, req UpdateOrganizationRequest) (Organization, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s", orgID), req)
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

// DeleteOrganization will remove the corresponding organization from the deployment, based on
// the UUID/name provided as `orgID`.
func (c *Client) DeleteOrganization(ctx context.Context, orgID string) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/organizations/%s", orgID), nil)
	if err != nil {
		return xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}

	return nil
}

// ProvisionerDaemons returns provisioner daemons available.
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

func (c *Client) OrganizationProvisionerDaemons(ctx context.Context, organizationID uuid.UUID, tags map[string]string) ([]ProvisionerDaemon, error) {
	baseURL := fmt.Sprintf("/api/v2/organizations/%s/provisionerdaemons", organizationID.String())

	queryParams := url.Values{}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return nil, xerrors.Errorf("marshal tags: %w", err)
	}

	queryParams.Add("tags", string(tagsJSON))
	if len(queryParams) > 0 {
		baseURL = fmt.Sprintf("%s?%s", baseURL, queryParams.Encode())
	}

	res, err := c.Request(ctx, http.MethodGet, baseURL, nil)
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

type OrganizationProvisionerJobsOptions struct {
	Limit  int
	IDs    []uuid.UUID
	Status []ProvisionerJobStatus
	Tags   map[string]string
}

func (c *Client) OrganizationProvisionerJobs(ctx context.Context, organizationID uuid.UUID, opts *OrganizationProvisionerJobsOptions) ([]ProvisionerJob, error) {
	qp := url.Values{}
	if opts != nil {
		if opts.Limit > 0 {
			qp.Add("limit", strconv.Itoa(opts.Limit))
		}
		if len(opts.IDs) > 0 {
			qp.Add("ids", joinSliceStringer(opts.IDs))
		}
		if len(opts.Status) > 0 {
			qp.Add("status", joinSlice(opts.Status))
		}
		if len(opts.Tags) > 0 {
			tagsRaw, err := json.Marshal(opts.Tags)
			if err != nil {
				return nil, xerrors.Errorf("marshal tags: %w", err)
			}
			qp.Add("tags", string(tagsRaw))
		}
	}

	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/provisionerjobs?%s", organizationID.String(), qp.Encode()),
		nil,
	)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var jobs []ProvisionerJob
	return jobs, json.NewDecoder(res.Body).Decode(&jobs)
}

func (c *Client) OrganizationProvisionerJob(ctx context.Context, organizationID, jobID uuid.UUID) (job ProvisionerJob, err error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/provisionerjobs/%s", organizationID.String(), jobID.String()),
		nil,
	)
	if err != nil {
		return job, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return job, ReadBodyAsError(res)
	}
	return job, json.NewDecoder(res.Body).Decode(&job)
}

func joinSlice[T ~string](s []T) string {
	var ss []string
	for _, v := range s {
		ss = append(ss, string(v))
	}
	return strings.Join(ss, ",")
}

func joinSliceStringer[T fmt.Stringer](s []T) string {
	var ss []string
	for _, v := range s {
		ss = append(ss, v.String())
	}
	return strings.Join(ss, ",")
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

type TemplateFilter struct {
	OrganizationID uuid.UUID `typescript:"-"`
	ExactName      string    `typescript:"-"`
	FuzzyName      string    `typescript:"-"`
	SearchQuery    string    `json:"q,omitempty"`
}

// asRequestOption returns a function that can be used in (*Client).Request.
// It modifies the request query parameters.
func (f TemplateFilter) asRequestOption() RequestOption {
	return func(r *http.Request) {
		var params []string
		// Make sure all user input is quoted to ensure it's parsed as a single
		// string.
		if f.OrganizationID != uuid.Nil {
			params = append(params, fmt.Sprintf("organization:%q", f.OrganizationID.String()))
		}

		if f.ExactName != "" {
			params = append(params, fmt.Sprintf("exact_name:%q", f.ExactName))
		}

		if f.FuzzyName != "" {
			params = append(params, fmt.Sprintf("name:%q", f.FuzzyName))
		}
		if f.SearchQuery != "" {
			params = append(params, f.SearchQuery)
		}

		q := r.URL.Query()
		q.Set("q", strings.Join(params, " "))
		r.URL.RawQuery = q.Encode()
	}
}

// Templates lists all viewable templates
func (c *Client) Templates(ctx context.Context, filter TemplateFilter) ([]Template, error) {
	res, err := c.Request(ctx, http.MethodGet,
		"/api/v2/templates",
		nil,
		filter.asRequestOption(),
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
//
// Deprecated: Use CreateUserWorkspace instead.
func (c *Client) CreateWorkspace(ctx context.Context, _ uuid.UUID, user string, request CreateWorkspaceRequest) (Workspace, error) {
	return c.CreateUserWorkspace(ctx, user, request)
}

// CreateUserWorkspace creates a new workspace for the template specified.
func (c *Client) CreateUserWorkspace(ctx context.Context, user string, request CreateWorkspaceRequest) (Workspace, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/users/%s/workspaces", user), request)
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
