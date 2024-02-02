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

	"github.com/coder/coder/v2/coderd/tracing"
)

type AutomaticUpdates string

const (
	AutomaticUpdatesAlways AutomaticUpdates = "always"
	AutomaticUpdatesNever  AutomaticUpdates = "never"
)

// Workspace is a deployment of a template. It references a specific
// version and can be updated.
type Workspace struct {
	ID                                   uuid.UUID      `json:"id" format:"uuid"`
	CreatedAt                            time.Time      `json:"created_at" format:"date-time"`
	UpdatedAt                            time.Time      `json:"updated_at" format:"date-time"`
	OwnerID                              uuid.UUID      `json:"owner_id" format:"uuid"`
	OwnerName                            string         `json:"owner_name"`
	OwnerAvatarURL                       string         `json:"owner_avatar_url"`
	OrganizationID                       uuid.UUID      `json:"organization_id" format:"uuid"`
	TemplateID                           uuid.UUID      `json:"template_id" format:"uuid"`
	TemplateName                         string         `json:"template_name"`
	TemplateDisplayName                  string         `json:"template_display_name"`
	TemplateIcon                         string         `json:"template_icon"`
	TemplateAllowUserCancelWorkspaceJobs bool           `json:"template_allow_user_cancel_workspace_jobs"`
	TemplateActiveVersionID              uuid.UUID      `json:"template_active_version_id" format:"uuid"`
	TemplateRequireActiveVersion         bool           `json:"template_require_active_version"`
	LatestBuild                          WorkspaceBuild `json:"latest_build"`
	Outdated                             bool           `json:"outdated"`
	Name                                 string         `json:"name"`
	AutostartSchedule                    *string        `json:"autostart_schedule,omitempty"`
	TTLMillis                            *int64         `json:"ttl_ms,omitempty"`
	LastUsedAt                           time.Time      `json:"last_used_at" format:"date-time"`

	// DeletingAt indicates the time at which the workspace will be permanently deleted.
	// A workspace is eligible for deletion if it is dormant (a non-nil dormant_at value)
	// and a value has been specified for time_til_dormant_autodelete on its template.
	DeletingAt *time.Time `json:"deleting_at" format:"date-time"`
	// DormantAt being non-nil indicates a workspace that is dormant.
	// A dormant workspace is no longer accessible must be activated.
	// It is subject to deletion if it breaches
	// the duration of the time_til_ field on its template.
	DormantAt *time.Time `json:"dormant_at" format:"date-time"`
	// Health shows the health of the workspace and information about
	// what is causing an unhealthy status.
	Health           WorkspaceHealth  `json:"health"`
	AutomaticUpdates AutomaticUpdates `json:"automatic_updates" enums:"always,never"`
	AllowRenames     bool             `json:"allow_renames"`
	Favorite         bool             `json:"favorite"`
}

func (w Workspace) FullName() string {
	return fmt.Sprintf("%s/%s", w.OwnerName, w.Name)
}

type WorkspaceHealth struct {
	Healthy       bool        `json:"healthy" example:"false"`      // Healthy is true if the workspace is healthy.
	FailingAgents []uuid.UUID `json:"failing_agents" format:"uuid"` // FailingAgents lists the IDs of the agents that are failing, if any.
}

type WorkspacesRequest struct {
	SearchQuery string `json:"q,omitempty"`
	Pagination
}

type WorkspacesResponse struct {
	Workspaces []Workspace `json:"workspaces"`
	Count      int         `json:"count"`
}

type ProvisionerLogLevel string

const (
	ProvisionerLogLevelDebug ProvisionerLogLevel = "debug"
)

// CreateWorkspaceBuildRequest provides options to update the latest workspace build.
type CreateWorkspaceBuildRequest struct {
	TemplateVersionID uuid.UUID           `json:"template_version_id,omitempty" format:"uuid"`
	Transition        WorkspaceTransition `json:"transition" validate:"oneof=create start stop delete,required"`
	DryRun            bool                `json:"dry_run,omitempty"`
	ProvisionerState  []byte              `json:"state,omitempty"`
	// Orphan may be set for the Destroy transition.
	Orphan bool `json:"orphan,omitempty"`
	// ParameterValues are optional. It will write params to the 'workspace' scope.
	// This will overwrite any existing parameters with the same name.
	// This will not delete old params not included in this list.
	RichParameterValues []WorkspaceBuildParameter `json:"rich_parameter_values,omitempty"`

	// Log level changes the default logging verbosity of a provider ("info" if empty).
	LogLevel ProvisionerLogLevel `json:"log_level,omitempty" validate:"omitempty,oneof=debug"`
}

type WorkspaceOptions struct {
	IncludeDeleted bool `json:"include_deleted,omitempty"`
}

// asRequestOption returns a function that can be used in (*Client).Request.
// It modifies the request query parameters.
func (o WorkspaceOptions) asRequestOption() RequestOption {
	return func(r *http.Request) {
		q := r.URL.Query()
		if o.IncludeDeleted {
			q.Set("include_deleted", "true")
		}
		r.URL.RawQuery = q.Encode()
	}
}

// Workspace returns a single workspace.
func (c *Client) Workspace(ctx context.Context, id uuid.UUID) (Workspace, error) {
	return c.getWorkspace(ctx, id)
}

// DeletedWorkspace returns a single workspace that was deleted.
func (c *Client) DeletedWorkspace(ctx context.Context, id uuid.UUID) (Workspace, error) {
	o := WorkspaceOptions{
		IncludeDeleted: true,
	}
	return c.getWorkspace(ctx, id, o.asRequestOption())
}

func (c *Client) getWorkspace(ctx context.Context, id uuid.UUID, opts ...RequestOption) (Workspace, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s", id), nil, opts...)
	if err != nil {
		return Workspace{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Workspace{}, ReadBodyAsError(res)
	}
	var workspace Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}

type WorkspaceBuildsRequest struct {
	WorkspaceID uuid.UUID `json:"workspace_id" format:"uuid" typescript:"-"`
	Pagination
	Since time.Time `json:"since,omitempty" format:"date-time"`
}

func (c *Client) WorkspaceBuilds(ctx context.Context, req WorkspaceBuildsRequest) ([]WorkspaceBuild, error) {
	res, err := c.Request(
		ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/workspaces/%s/builds", req.WorkspaceID),
		nil, req.Pagination.asRequestOption(), WithQueryParam("since", req.Since.Format(time.RFC3339)),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var workspaceBuild []WorkspaceBuild
	return workspaceBuild, json.NewDecoder(res.Body).Decode(&workspaceBuild)
}

// CreateWorkspaceBuild queues a new build to occur for a workspace.
func (c *Client) CreateWorkspaceBuild(ctx context.Context, workspace uuid.UUID, request CreateWorkspaceBuildRequest) (WorkspaceBuild, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/workspaces/%s/builds", workspace), request)
	if err != nil {
		return WorkspaceBuild{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return WorkspaceBuild{}, ReadBodyAsError(res)
	}
	var workspaceBuild WorkspaceBuild
	return workspaceBuild, json.NewDecoder(res.Body).Decode(&workspaceBuild)
}

func (c *Client) WatchWorkspace(ctx context.Context, id uuid.UUID) (<-chan Workspace, error) {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()
	//nolint:bodyclose
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/watch", id), nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	nextEvent := ServerSentEventReader(ctx, res.Body)

	wc := make(chan Workspace, 256)
	go func() {
		defer close(wc)
		defer res.Body.Close()

		for {
			select {
			case <-ctx.Done():
				return
			default:
				sse, err := nextEvent()
				if err != nil {
					return
				}
				if sse.Type != ServerSentEventTypeData {
					continue
				}
				var ws Workspace
				b, ok := sse.Data.([]byte)
				if !ok {
					return
				}
				err = json.Unmarshal(b, &ws)
				if err != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case wc <- ws:
				}
			}
		}
	}()

	return wc, nil
}

type UpdateWorkspaceRequest struct {
	Name string `json:"name,omitempty" validate:"username"`
}

func (c *Client) UpdateWorkspace(ctx context.Context, id uuid.UUID, req UpdateWorkspaceRequest) error {
	path := fmt.Sprintf("/api/v2/workspaces/%s", id.String())
	res, err := c.Request(ctx, http.MethodPatch, path, req)
	if err != nil {
		return xerrors.Errorf("update workspace: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// UpdateWorkspaceAutostartRequest is a request to update a workspace's autostart schedule.
type UpdateWorkspaceAutostartRequest struct {
	Schedule *string `json:"schedule"`
}

// UpdateWorkspaceAutostart sets the autostart schedule for workspace by id.
// If the provided schedule is empty, autostart is disabled for the workspace.
func (c *Client) UpdateWorkspaceAutostart(ctx context.Context, id uuid.UUID, req UpdateWorkspaceAutostartRequest) error {
	path := fmt.Sprintf("/api/v2/workspaces/%s/autostart", id.String())
	res, err := c.Request(ctx, http.MethodPut, path, req)
	if err != nil {
		return xerrors.Errorf("update workspace autostart: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// UpdateWorkspaceTTLRequest is a request to update a workspace's TTL.
type UpdateWorkspaceTTLRequest struct {
	TTLMillis *int64 `json:"ttl_ms"`
}

// UpdateWorkspaceTTL sets the ttl for workspace by id.
// If the provided duration is nil, autostop is disabled for the workspace.
func (c *Client) UpdateWorkspaceTTL(ctx context.Context, id uuid.UUID, req UpdateWorkspaceTTLRequest) error {
	path := fmt.Sprintf("/api/v2/workspaces/%s/ttl", id.String())
	res, err := c.Request(ctx, http.MethodPut, path, req)
	if err != nil {
		return xerrors.Errorf("update workspace time until shutdown: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// PutExtendWorkspaceRequest is a request to extend the deadline of
// the active workspace build.
type PutExtendWorkspaceRequest struct {
	Deadline time.Time `json:"deadline" validate:"required" format:"date-time"`
}

// PutExtendWorkspace updates the deadline for resources of the latest workspace build.
func (c *Client) PutExtendWorkspace(ctx context.Context, id uuid.UUID, req PutExtendWorkspaceRequest) error {
	path := fmt.Sprintf("/api/v2/workspaces/%s/extend", id.String())
	res, err := c.Request(ctx, http.MethodPut, path, req)
	if err != nil {
		return xerrors.Errorf("extend workspace time until shutdown: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNotModified {
		return ReadBodyAsError(res)
	}
	return nil
}

// UpdateWorkspaceDormancy is a request to activate or make a workspace dormant.
// A value of false will activate a dormant workspace.
type UpdateWorkspaceDormancy struct {
	Dormant bool `json:"dormant"`
}

// UpdateWorkspaceDormancy sets a workspace as dormant if dormant=true and activates a dormant workspace
// if dormant=false.
func (c *Client) UpdateWorkspaceDormancy(ctx context.Context, id uuid.UUID, req UpdateWorkspaceDormancy) error {
	path := fmt.Sprintf("/api/v2/workspaces/%s/dormant", id.String())
	res, err := c.Request(ctx, http.MethodPut, path, req)
	if err != nil {
		return xerrors.Errorf("update workspace lock: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNotModified {
		return ReadBodyAsError(res)
	}
	return nil
}

// UpdateWorkspaceAutomaticUpdatesRequest is a request to updates a workspace's automatic updates setting.
type UpdateWorkspaceAutomaticUpdatesRequest struct {
	AutomaticUpdates AutomaticUpdates `json:"automatic_updates"`
}

// UpdateWorkspaceAutomaticUpdates sets the automatic updates setting for workspace by id.
func (c *Client) UpdateWorkspaceAutomaticUpdates(ctx context.Context, id uuid.UUID, req UpdateWorkspaceAutomaticUpdatesRequest) error {
	path := fmt.Sprintf("/api/v2/workspaces/%s/autoupdates", id.String())
	res, err := c.Request(ctx, http.MethodPut, path, req)
	if err != nil {
		return xerrors.Errorf("update workspace automatic updates: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

type WorkspaceFilter struct {
	// Owner can be "me" or a username
	Owner string `json:"owner,omitempty" typescript:"-"`
	// Template is a template name
	Template string `json:"template,omitempty" typescript:"-"`
	// Name will return partial matches
	Name string `json:"name,omitempty" typescript:"-"`
	// Status is a workspace status, which is really the status of the latest build
	Status string `json:"status,omitempty" typescript:"-"`
	// Offset is the number of workspaces to skip before returning results.
	Offset int `json:"offset,omitempty" typescript:"-"`
	// Limit is a limit on the number of workspaces returned.
	Limit int `json:"limit,omitempty" typescript:"-"`
	// FilterQuery supports a raw filter query string
	FilterQuery string `json:"q,omitempty"`
}

// asRequestOption returns a function that can be used in (*Client).Request.
// It modifies the request query parameters.
func (f WorkspaceFilter) asRequestOption() RequestOption {
	return func(r *http.Request) {
		var params []string
		// Make sure all user input is quoted to ensure it's parsed as a single
		// string.
		if f.Owner != "" {
			params = append(params, fmt.Sprintf("owner:%q", f.Owner))
		}
		if f.Name != "" {
			params = append(params, fmt.Sprintf("name:%q", f.Name))
		}
		if f.Template != "" {
			params = append(params, fmt.Sprintf("template:%q", f.Template))
		}
		if f.Status != "" {
			params = append(params, fmt.Sprintf("status:%q", f.Status))
		}
		if f.FilterQuery != "" {
			// If custom stuff is added, just add it on here.
			params = append(params, f.FilterQuery)
		}

		q := r.URL.Query()
		q.Set("q", strings.Join(params, " "))
		r.URL.RawQuery = q.Encode()
	}
}

// Workspaces returns all workspaces the authenticated user has access to.
func (c *Client) Workspaces(ctx context.Context, filter WorkspaceFilter) (WorkspacesResponse, error) {
	page := Pagination{
		Offset: filter.Offset,
		Limit:  filter.Limit,
	}
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/workspaces", nil, filter.asRequestOption(), page.asRequestOption())
	if err != nil {
		return WorkspacesResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return WorkspacesResponse{}, ReadBodyAsError(res)
	}

	var wres WorkspacesResponse
	return wres, json.NewDecoder(res.Body).Decode(&wres)
}

// WorkspaceByOwnerAndName returns a workspace by the owner's UUID and the workspace's name.
func (c *Client) WorkspaceByOwnerAndName(ctx context.Context, owner string, name string, params WorkspaceOptions) (Workspace, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/users/%s/workspace/%s", owner, name), nil, func(r *http.Request) {
		q := r.URL.Query()
		q.Set("include_deleted", fmt.Sprintf("%t", params.IncludeDeleted))
		r.URL.RawQuery = q.Encode()
	})
	if err != nil {
		return Workspace{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Workspace{}, ReadBodyAsError(res)
	}

	var workspace Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}

type WorkspaceQuota struct {
	CreditsConsumed int `json:"credits_consumed"`
	Budget          int `json:"budget"`
}

func (c *Client) WorkspaceQuota(ctx context.Context, userID string) (WorkspaceQuota, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspace-quota/%s", userID), nil)
	if err != nil {
		return WorkspaceQuota{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceQuota{}, ReadBodyAsError(res)
	}
	var quota WorkspaceQuota
	return quota, json.NewDecoder(res.Body).Decode(&quota)
}

type ResolveAutostartResponse struct {
	ParameterMismatch bool `json:"parameter_mismatch"`
}

func (c *Client) ResolveAutostart(ctx context.Context, workspaceID string) (ResolveAutostartResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/resolve-autostart", workspaceID), nil)
	if err != nil {
		return ResolveAutostartResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ResolveAutostartResponse{}, ReadBodyAsError(res)
	}
	var response ResolveAutostartResponse
	return response, json.NewDecoder(res.Body).Decode(&response)
}

func (c *Client) FavoriteWorkspace(ctx context.Context, workspaceID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/workspaces/%s/favorite", workspaceID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

func (c *Client) UnfavoriteWorkspace(ctx context.Context, workspaceID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/workspaces/%s/favorite", workspaceID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// WorkspaceNotifyChannel is the PostgreSQL NOTIFY
// channel to listen for updates on. The payload is empty,
// because the size of a workspace payload can be very large.
func WorkspaceNotifyChannel(id uuid.UUID) string {
	return fmt.Sprintf("workspace:%s", id)
}
