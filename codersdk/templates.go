package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	ActiveUserCount    int                    `json:"active_user_count"`
	BuildTimeStats     TemplateBuildTimeStats `json:"build_time_stats"`
	Description        string                 `json:"description"`
	Deprecated         bool                   `json:"deprecated"`
	DeprecationMessage string                 `json:"deprecation_message"`
	Icon               string                 `json:"icon"`
	DefaultTTLMillis   int64                  `json:"default_ttl_ms"`
	// TODO(@dean): remove max_ttl once autostop_requirement is matured
	MaxTTLMillis int64 `json:"max_ttl_ms"`
	// AutostopRequirement and AutostartRequirement are enterprise features. Its
	// value is only used if your license is entitled to use the advanced template
	// scheduling feature.
	AutostopRequirement  TemplateAutostopRequirement  `json:"autostop_requirement"`
	AutostartRequirement TemplateAutostartRequirement `json:"autostart_requirement"`
	CreatedByID          uuid.UUID                    `json:"created_by_id" format:"uuid"`
	CreatedByName        string                       `json:"created_by_name"`

	// AllowUserAutostart and AllowUserAutostop are enterprise-only. Their
	// values are only used if your license is entitled to use the advanced
	// template scheduling feature.
	AllowUserAutostart           bool `json:"allow_user_autostart"`
	AllowUserAutostop            bool `json:"allow_user_autostop"`
	AllowUserCancelWorkspaceJobs bool `json:"allow_user_cancel_workspace_jobs"`

	// FailureTTLMillis, TimeTilDormantMillis, and TimeTilDormantAutoDeleteMillis are enterprise-only. Their
	// values are used if your license is entitled to use the advanced
	// template scheduling feature.
	FailureTTLMillis               int64 `json:"failure_ttl_ms"`
	TimeTilDormantMillis           int64 `json:"time_til_dormant_ms"`
	TimeTilDormantAutoDeleteMillis int64 `json:"time_til_dormant_autodelete_ms"`

	// RequireActiveVersion mandates that workspaces are built with the active
	// template version.
	RequireActiveVersion bool `json:"require_active_version"`
}

// WeekdaysToBitmap converts a list of weekdays to a bitmap in accordance with
// the schedule package's rules. The 0th bit is Monday, ..., the 6th bit is
// Sunday. The 7th bit is unused.
func WeekdaysToBitmap(days []string) (uint8, error) {
	var bitmap uint8
	for _, day := range days {
		switch strings.ToLower(day) {
		case "monday":
			bitmap |= 1 << 0
		case "tuesday":
			bitmap |= 1 << 1
		case "wednesday":
			bitmap |= 1 << 2
		case "thursday":
			bitmap |= 1 << 3
		case "friday":
			bitmap |= 1 << 4
		case "saturday":
			bitmap |= 1 << 5
		case "sunday":
			bitmap |= 1 << 6
		default:
			return 0, xerrors.Errorf("invalid weekday %q", day)
		}
	}
	return bitmap, nil
}

// BitmapToWeekdays converts a bitmap to a list of weekdays in accordance with
// the schedule package's rules (see above).
func BitmapToWeekdays(bitmap uint8) []string {
	days := []string{}
	for i := 0; i < 7; i++ {
		if bitmap&(1<<i) != 0 {
			switch i {
			case 0:
				days = append(days, "monday")
			case 1:
				days = append(days, "tuesday")
			case 2:
				days = append(days, "wednesday")
			case 3:
				days = append(days, "thursday")
			case 4:
				days = append(days, "friday")
			case 5:
				days = append(days, "saturday")
			case 6:
				days = append(days, "sunday")
			}
		}
	}
	return days
}

type TemplateAutostartRequirement struct {
	// DaysOfWeek is a list of days of the week in which autostart is allowed
	// to happen. If no days are specified, autostart is not allowed.
	DaysOfWeek []string `json:"days_of_week" enums:"monday,tuesday,wednesday,thursday,friday,saturday,sunday"`
}

type TemplateAutostopRequirement struct {
	// DaysOfWeek is a list of days of the week on which restarts are required.
	// Restarts happen within the user's quiet hours (in their configured
	// timezone). If no days are specified, restarts are not required. Weekdays
	// cannot be specified twice.
	//
	// Restarts will only happen on weekdays in this list on weeks which line up
	// with Weeks.
	DaysOfWeek []string `json:"days_of_week" enums:"monday,tuesday,wednesday,thursday,friday,saturday,sunday"`
	// Weeks is the number of weeks between required restarts. Weeks are synced
	// across all workspaces (and Coder deployments) using modulo math on a
	// hardcoded epoch week of January 2nd, 2023 (the first Monday of 2023).
	// Values of 0 or 1 indicate weekly restarts. Values of 2 indicate
	// fortnightly restarts, etc.
	Weeks int64 `json:"weeks"`
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

type ArchiveTemplateVersionsRequest struct {
	// By default, only failed versions are archived. Set this to true
	// to archive all unused versions regardless of job status.
	All bool `json:"all"`
}

type ArchiveTemplateVersionsResponse struct {
	TemplateID  uuid.UUID   `json:"template_id" format:"uuid"`
	ArchivedIDs []uuid.UUID `json:"archived_ids"`
}

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
	// UserPerms should be a mapping of user id to role. The user id must be the
	// uuid of the user, not a username or email address.
	UserPerms map[string]TemplateRole `json:"user_perms,omitempty" example:"<group_id>:admin,4df59e74-c027-470b-ab4d-cbba8963a5e9:use"`
	// GroupPerms should be a mapping of group id to role.
	GroupPerms map[string]TemplateRole `json:"group_perms,omitempty" example:"<user_id>>:admin,8bd26b20-f3e8-48be-a903-46bb920cf671:use"`
}

// ACLAvailable is a list of users and groups that can be added to a template
// ACL.
type ACLAvailable struct {
	Users  []User  `json:"users"`
	Groups []Group `json:"groups"`
}

type UpdateTemplateMeta struct {
	Name             string `json:"name,omitempty" validate:"omitempty,template_name"`
	DisplayName      string `json:"display_name,omitempty" validate:"omitempty,template_display_name"`
	Description      string `json:"description,omitempty"`
	Icon             string `json:"icon,omitempty"`
	DefaultTTLMillis int64  `json:"default_ttl_ms,omitempty"`
	// TODO(@dean): remove max_ttl once autostop_requirement is matured
	MaxTTLMillis int64 `json:"max_ttl_ms,omitempty"`
	// AutostopRequirement and AutostartRequirement can only be set if your license
	// includes the advanced template scheduling feature. If you attempt to set this
	// value while unlicensed, it will be ignored.
	AutostopRequirement            *TemplateAutostopRequirement  `json:"autostop_requirement,omitempty"`
	AutostartRequirement           *TemplateAutostartRequirement `json:"autostart_requirement,omitempty"`
	AllowUserAutostart             bool                          `json:"allow_user_autostart,omitempty"`
	AllowUserAutostop              bool                          `json:"allow_user_autostop,omitempty"`
	AllowUserCancelWorkspaceJobs   bool                          `json:"allow_user_cancel_workspace_jobs,omitempty"`
	FailureTTLMillis               int64                         `json:"failure_ttl_ms,omitempty"`
	TimeTilDormantMillis           int64                         `json:"time_til_dormant_ms,omitempty"`
	TimeTilDormantAutoDeleteMillis int64                         `json:"time_til_dormant_autodelete_ms,omitempty"`
	// UpdateWorkspaceLastUsedAt updates the last_used_at field of workspaces
	// spawned from the template. This is useful for preventing workspaces being
	// immediately locked when updating the inactivity_ttl field to a new, shorter
	// value.
	UpdateWorkspaceLastUsedAt bool `json:"update_workspace_last_used_at"`
	// UpdateWorkspaceDormant updates the dormant_at field of workspaces spawned
	// from the template. This is useful for preventing dormant workspaces being immediately
	// deleted when updating the dormant_ttl field to a new, shorter value.
	UpdateWorkspaceDormantAt bool `json:"update_workspace_dormant_at"`
	// RequireActiveVersion mandates workspaces built using this template
	// use the active version of the template. This option has no
	// effect on template admins.
	RequireActiveVersion bool `json:"require_active_version"`
	// DeprecationMessage if set, will mark the template as deprecated and block
	// any new workspaces from using this template.
	// If passed an empty string, will remove the deprecated message, making
	// the template usable for new workspaces again.
	DeprecationMessage *string `json:"deprecation_message"`
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

func (c *Client) ArchiveTemplateVersions(ctx context.Context, template uuid.UUID, all bool) (ArchiveTemplateVersionsResponse, error) {
	res, err := c.Request(ctx, http.MethodPost,
		fmt.Sprintf("/api/v2/templates/%s/versions/archive", template),
		ArchiveTemplateVersionsRequest{
			All: all,
		},
	)
	if err != nil {
		return ArchiveTemplateVersionsResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ArchiveTemplateVersionsResponse{}, ReadBodyAsError(res)
	}
	var resp ArchiveTemplateVersionsResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

//nolint:revive
func (c *Client) SetArchiveTemplateVersion(ctx context.Context, templateVersion uuid.UUID, archive bool) error {
	u := fmt.Sprintf("/api/v2/templateversions/%s", templateVersion.String())
	if archive {
		u += "/archive"
	} else {
		u += "/unarchive"
	}
	res, err := c.Request(ctx, http.MethodPost, u, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}

	return nil
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

// TemplateACLAvailable returns available users + groups that can be assigned template perms
func (c *Client) TemplateACLAvailable(ctx context.Context, templateID uuid.UUID) (ACLAvailable, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/templates/%s/acl/available", templateID), nil)
	if err != nil {
		return ACLAvailable{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ACLAvailable{}, ReadBodyAsError(res)
	}
	var acl ACLAvailable
	return acl, json.NewDecoder(res.Body).Decode(&acl)
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
	TemplateID      uuid.UUID `json:"template_id" validate:"required" format:"uuid"`
	IncludeArchived bool      `json:"include_archived"`
	Pagination
}

// TemplateVersionsByTemplate lists versions associated with a template.
func (c *Client) TemplateVersionsByTemplate(ctx context.Context, req TemplateVersionsByTemplateRequest) ([]TemplateVersion, error) {
	u := fmt.Sprintf("/api/v2/templates/%s/versions", req.TemplateID)
	if req.IncludeArchived {
		u += "?include_archived=true"
	}
	res, err := c.Request(ctx, http.MethodGet, u, nil, req.Pagination.asRequestOption())
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
