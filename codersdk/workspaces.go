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

// Workspace is a deployment of a template. It references a specific
// version and can be updated.
type Workspace struct {
	ID                uuid.UUID      `json:"id"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	OwnerID           uuid.UUID      `json:"owner_id"`
	OwnerName         string         `json:"owner_name"`
	TemplateID        uuid.UUID      `json:"template_id"`
	TemplateName      string         `json:"template_name"`
	TemplateIcon      string         `json:"template_icon"`
	LatestBuild       WorkspaceBuild `json:"latest_build"`
	Outdated          bool           `json:"outdated"`
	Name              string         `json:"name"`
	AutostartSchedule *string        `json:"autostart_schedule,omitempty"`
	TTLMillis         *int64         `json:"ttl_ms,omitempty"`
	LastUsedAt        time.Time      `json:"last_used_at"`
}

// CreateWorkspaceBuildRequest provides options to update the latest workspace build.
type CreateWorkspaceBuildRequest struct {
	TemplateVersionID uuid.UUID           `json:"template_version_id,omitempty"`
	Transition        WorkspaceTransition `json:"transition" validate:"oneof=create start stop delete,required"`
	DryRun            bool                `json:"dry_run,omitempty"`
	ProvisionerState  []byte              `json:"state,omitempty"`
	// Orphan may be set for the Destroy transition.
	Orphan bool `json:"orphan,omitempty"`
	// ParameterValues are optional. It will write params to the 'workspace' scope.
	// This will overwrite any existing parameters with the same name.
	// This will not delete old params not included in this list.
	ParameterValues []CreateParameterRequest `json:"parameter_values,omitempty"`
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
		return Workspace{}, readBodyAsError(res)
	}
	var workspace Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}

type WorkspaceBuildsRequest struct {
	WorkspaceID uuid.UUID
	Pagination
	Since time.Time
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
		return nil, readBodyAsError(res)
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
		return WorkspaceBuild{}, readBodyAsError(res)
	}
	var workspaceBuild WorkspaceBuild
	return workspaceBuild, json.NewDecoder(res.Body).Decode(&workspaceBuild)
}

func (c *Client) WatchWorkspace(ctx context.Context, id uuid.UUID) (<-chan Workspace, error) {
	//nolint:bodyclose
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/watch", id), nil)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}
	nextEvent := ServerSentEventReader(res.Body)

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
				if sse.Type == ServerSentEventTypeData {
					var ws Workspace
					b, ok := sse.Data.([]byte)
					if !ok {
						return
					}
					err = json.Unmarshal(b, &ws)
					if err != nil {
						return
					}
					wc <- ws
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
		return readBodyAsError(res)
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
		return readBodyAsError(res)
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
		return readBodyAsError(res)
	}
	return nil
}

// PutExtendWorkspaceRequest is a request to extend the deadline of
// the active workspace build.
type PutExtendWorkspaceRequest struct {
	Deadline time.Time `json:"deadline" validate:"required"`
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
		return readBodyAsError(res)
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
func (c *Client) Workspaces(ctx context.Context, filter WorkspaceFilter) ([]Workspace, error) {
	page := Pagination{
		Offset: filter.Offset,
		Limit:  filter.Limit,
	}
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/workspaces", nil, filter.asRequestOption(), page.asRequestOption())
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, readBodyAsError(res)
	}

	var workspaces []Workspace
	return workspaces, json.NewDecoder(res.Body).Decode(&workspaces)
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
		return Workspace{}, readBodyAsError(res)
	}

	var workspace Workspace
	return workspace, json.NewDecoder(res.Body).Decode(&workspace)
}

type GetAppHostResponse struct {
	Host string `json:"host"`
}

// GetAppHost returns the site-wide application wildcard hostname without the
// leading "*.", e.g. "apps.coder.com". Apps are accessible at:
// "<app-name>--<agent-name>--<workspace-name>--<username>.<app-host>", e.g.
// "my-app--agent--workspace--username.apps.coder.com".
//
// If the app host is not set, the response will contain an empty string.
func (c *Client) GetAppHost(ctx context.Context) (GetAppHostResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/applications/host", nil)
	if err != nil {
		return GetAppHostResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GetAppHostResponse{}, readBodyAsError(res)
	}

	var host GetAppHostResponse
	return host, json.NewDecoder(res.Body).Decode(&host)
}
