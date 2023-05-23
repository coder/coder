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

// Template is the JSON representation of a Coder template. This type matches the
// database object for now, but is abstracted for ease of change later on.
type Template struct {
	ID              uuid.UUID       `json:"id" format:"uuid"`
	CreatedAt       time.Time       `json:"created_at" format:"date-time"`
	UpdatedAt       time.Time       `json:"updated_at" format:"date-time"`
	OrganizationID  uuid.UUID       `json:"organization_id" format:"uuid"`
	Name            string          `json:"name"`
	DisplayName     string          `json:"display_name"`
	Provisioner     ProvisionerType `json:"provisioner" enums:"terraform"`
	ActiveVersionID uuid.UUID       `json:"active_version_id" format:"uuid"`
	// ActiveUserCount is set to -1 when loading.
	ActiveUserCount  int                    `json:"active_user_count"`
	BuildTimeStats   TemplateBuildTimeStats `json:"build_time_stats"`
	Description      string                 `json:"description"`
	Icon             string                 `json:"icon"`
	DefaultTTLMillis int64                  `json:"default_ttl_ms"`
	// MaxTTLMillis is an enterprise feature. It's value is only used if your
	// license is entitled to use the advanced template scheduling feature.
	MaxTTLMillis  int64     `json:"max_ttl_ms"`
	CreatedByID   uuid.UUID `json:"created_by_id" format:"uuid"`
	CreatedByName string    `json:"created_by_name"`

	// AllowUserAutostart and AllowUserAutostop are enterprise-only. Their
	// values are only used if your license is entitled to use the advanced
	// template scheduling feature.
	AllowUserAutostart           bool `json:"allow_user_autostart"`
	AllowUserAutostop            bool `json:"allow_user_autostop"`
	AllowUserCancelWorkspaceJobs bool `json:"allow_user_cancel_workspace_jobs"`

	// FailureTTLMillis and InactivityTTLMillis are enterprise-only. Their
	// values are used if your license is entitled to use the advanced
	// template scheduling feature.
	FailureTTLMillis    int64 `json:"failure_ttl_ms"`
	InactivityTTLMillis int64 `json:"inactivity_ttl_ms"`
}

type TransitionStats struct {
	P50 *int64 `example:"123"`
	P95 *int64 `example:"146"`
}

type (
	TemplateBuildTimeStats      map[WorkspaceTransition]TransitionStats
	UpdateActiveTemplateVersion struct {
		ID uuid.UUID `json:"id" validate:"required" format:"uuid"`
	}
)

type TemplateRole string

const (
	TemplateRoleAdmin   TemplateRole = "admin"
	TemplateRoleUse     TemplateRole = "use"
	TemplateRoleDeleted TemplateRole = ""
)

type TemplateACL struct {
	Users  []TemplateUser  `json:"users"`
	Groups []TemplateGroup `json:"group"`
}

type TemplateGroup struct {
	Group
	Role TemplateRole `json:"role" enums:"admin,use"`
}

type TemplateUser struct {
	User
	Role TemplateRole `json:"role" enums:"admin,use"`
}

type UpdateTemplateACL struct {
	UserPerms  map[string]TemplateRole `json:"user_perms,omitempty"`
	GroupPerms map[string]TemplateRole `json:"group_perms,omitempty"`
}

type UpdateTemplateMeta struct {
	Name             string `json:"name,omitempty" validate:"omitempty,template_name"`
	DisplayName      string `json:"display_name,omitempty" validate:"omitempty,template_display_name"`
	Description      string `json:"description,omitempty"`
	Icon             string `json:"icon,omitempty"`
	DefaultTTLMillis int64  `json:"default_ttl_ms,omitempty"`
	// MaxTTLMillis can only be set if your license includes the advanced
	// template scheduling feature. If you attempt to set this value while
	// unlicensed, it will be ignored.
	MaxTTLMillis                 int64 `json:"max_ttl_ms,omitempty"`
	AllowUserAutostart           bool  `json:"allow_user_autostart,omitempty"`
	AllowUserAutostop            bool  `json:"allow_user_autostop,omitempty"`
	AllowUserCancelWorkspaceJobs bool  `json:"allow_user_cancel_workspace_jobs,omitempty"`
	FailureTTLMillis             int64 `json:"failure_ttl_ms,omitempty"`
	InactivityTTLMillis          int64 `json:"inactivity_ttl_ms,omitempty"`
}

type TemplateExample struct {
	ID          string   `json:"id" format:"uuid"`
	URL         string   `json:"url"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Tags        []string `json:"tags"`
	Markdown    string   `json:"markdown"`
}

// Template returns a single template.
func (c *Client) Template(ctx context.Context, template uuid.UUID) (Template, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s", template), nil)
	if err != nil {
		return Template{}, nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Template{}, ReadBodyAsError(res)
	}
	var resp Template
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) DeleteTemplate(ctx context.Context, template uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/templates/%s", template), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

func (c *Client) UpdateTemplateMeta(ctx context.Context, templateID uuid.UUID, req UpdateTemplateMeta) (Template, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/templates/%s", templateID), req)
	if err != nil {
		return Template{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotModified {
		return Template{}, xerrors.New("template metadata not modified")
	}
	if res.StatusCode != http.StatusOK {
		return Template{}, ReadBodyAsError(res)
	}
	var updated Template
	return updated, json.NewDecoder(res.Body).Decode(&updated)
}

func (c *Client) UpdateTemplateACL(ctx context.Context, templateID uuid.UUID, req UpdateTemplateACL) error {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/templates/%s/acl", templateID), req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

func (c *Client) TemplateACL(ctx context.Context, templateID uuid.UUID) (TemplateACL, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s/acl", templateID), nil)
	if err != nil {
		return TemplateACL{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return TemplateACL{}, ReadBodyAsError(res)
	}
	var acl TemplateACL
	return acl, json.NewDecoder(res.Body).Decode(&acl)
}

// UpdateActiveTemplateVersion updates the active template version to the ID provided.
// The template version must be attached to the template.
func (c *Client) UpdateActiveTemplateVersion(ctx context.Context, template uuid.UUID, req UpdateActiveTemplateVersion) error {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/templates/%s/versions", template), req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

// TemplateVersionsByTemplateRequest defines the request parameters for
// TemplateVersionsByTemplate.
type TemplateVersionsByTemplateRequest struct {
	TemplateID uuid.UUID `json:"template_id" validate:"required" format:"uuid"`
	Pagination
}

// TemplateVersionsByTemplate lists versions associated with a template.
func (c *Client) TemplateVersionsByTemplate(ctx context.Context, req TemplateVersionsByTemplateRequest) ([]TemplateVersion, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s/versions", req.TemplateID), nil, req.Pagination.asRequestOption())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var templateVersion []TemplateVersion
	return templateVersion, json.NewDecoder(res.Body).Decode(&templateVersion)
}

// TemplateVersionByName returns a template version by it's friendly name.
// This is used for path-based routing. Like: /templates/example/versions/helloworld
func (c *Client) TemplateVersionByName(ctx context.Context, template uuid.UUID, name string) (TemplateVersion, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s/versions/%s", template, name), nil)
	if err != nil {
		return TemplateVersion{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return TemplateVersion{}, ReadBodyAsError(res)
	}
	var templateVersion TemplateVersion
	return templateVersion, json.NewDecoder(res.Body).Decode(&templateVersion)
}

func (c *Client) TemplateDAUsLocalTZ(ctx context.Context, templateID uuid.UUID) (*DAUsResponse, error) {
	return c.TemplateDAUs(ctx, templateID, TimezoneOffsetHour(time.Local))
}

// TemplateDAUs requires a tzOffset in hours. Use 0 for UTC, and TimezoneOffsetHour(time.Local) for the
// local timezone.
func (c *Client) TemplateDAUs(ctx context.Context, templateID uuid.UUID, tzOffset int) (*DAUsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s/daus", templateID), nil, DAURequest{
		TZHourOffset: tzOffset,
	}.asRequestOption())
	if err != nil {
		return nil, xerrors.Errorf("execute request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var resp DAUsResponse
	return &resp, json.NewDecoder(res.Body).Decode(&resp)
}

// AgentStatsReportRequest is a WebSocket request by coderd
// to the agent for stats.
// @typescript-ignore AgentStatsReportRequest
type AgentStatsReportRequest struct{}

// AgentStatsReportResponse is returned for each report
// request by the agent.
type AgentStatsReportResponse struct {
	NumConns int64 `json:"num_comms"`
	// RxBytes is the number of received bytes.
	RxBytes int64 `json:"rx_bytes"`
	// TxBytes is the number of transmitted bytes.
	TxBytes int64 `json:"tx_bytes"`
}

// TemplateExamples lists example templates embedded in coder.
func (c *Client) TemplateExamples(ctx context.Context, organizationID uuid.UUID) ([]TemplateExample, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/templates/examples", organizationID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var templateExamples []TemplateExample
	return templateExamples, json.NewDecoder(res.Body).Decode(&templateExamples)
}
