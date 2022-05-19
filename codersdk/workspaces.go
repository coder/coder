package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/coder/coder/coderd/database"
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
	LatestBuild       WorkspaceBuild `json:"latest_build"`
	Outdated          bool           `json:"outdated"`
	Name              string         `json:"name"`
	AutostartSchedule string         `json:"autostart_schedule"`
	AutostopSchedule  string         `json:"autostop_schedule"`
}

// CreateWorkspaceBuildRequest provides options to update the latest workspace build.
type CreateWorkspaceBuildRequest struct {
	TemplateVersionID uuid.UUID                    `json:"template_version_id,omitempty"`
	Transition        database.WorkspaceTransition `json:"transition" validate:"oneof=create start stop delete,required"`
	DryRun            bool                         `json:"dry_run,omitempty"`
	ProvisionerState  []byte                       `json:"state,omitempty"`
}

// Workspace returns a single workspace.
func (c *Client) Workspace(ctx context.Context, id uuid.UUID) (Workspace, error) {
	return c.getWorkspace(ctx, id)
}

// DeletedWorkspace returns a single workspace that was deleted.
func (c *Client) DeletedWorkspace(ctx context.Context, id uuid.UUID) (Workspace, error) {
	return c.getWorkspace(ctx, id, queryParam("deleted", "true"))
}

func (c *Client) getWorkspace(ctx context.Context, id uuid.UUID, opts ...requestOption) (Workspace, error) {
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
}

func (c *Client) WorkspaceBuilds(ctx context.Context, req WorkspaceBuildsRequest) ([]WorkspaceBuild, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/builds", req.WorkspaceID),
		nil, req.Pagination.asRequestOption())
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

func (c *Client) WorkspaceBuildByName(ctx context.Context, workspace uuid.UUID, name string) (WorkspaceBuild, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/builds/%s", workspace, name), nil)
	if err != nil {
		return WorkspaceBuild{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceBuild{}, readBodyAsError(res)
	}
	var workspaceBuild WorkspaceBuild
	return workspaceBuild, json.NewDecoder(res.Body).Decode(&workspaceBuild)
}

func (c *Client) WatchWorkspace(ctx context.Context, id uuid.UUID) (<-chan Workspace, error) {
	conn, err := c.dialWebsocket(ctx, fmt.Sprintf("/api/v2/workspaces/%s/watch", id))
	if err != nil {
		return nil, err
	}
	wc := make(chan Workspace, 256)

	go func() {
		defer close(wc)
		defer conn.Close(websocket.StatusNormalClosure, "")

		for {
			select {
			case <-ctx.Done():
				return
			default:
				var ws Workspace
				err := wsjson.Read(ctx, conn, &ws)
				if err != nil {
					conn.Close(websocket.StatusInternalError, "failed to read workspace")
					return
				}
				wc <- ws
			}
		}
	}()

	return wc, nil
}

// UpdateWorkspaceAutostartRequest is a request to update a workspace's autostart schedule.
type UpdateWorkspaceAutostartRequest struct {
	Schedule string `json:"schedule"`
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
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}
	return nil
}

// UpdateWorkspaceAutostopRequest is a request to update a workspace's autostop schedule.
type UpdateWorkspaceAutostopRequest struct {
	Schedule string `json:"schedule"`
}

// UpdateWorkspaceAutostop sets the autostop schedule for workspace by id.
// If the provided schedule is empty, autostop is disabled for the workspace.
func (c *Client) UpdateWorkspaceAutostop(ctx context.Context, id uuid.UUID, req UpdateWorkspaceAutostopRequest) error {
	path := fmt.Sprintf("/api/v2/workspaces/%s/autostop", id.String())
	res, err := c.Request(ctx, http.MethodPut, path, req)
	if err != nil {
		return xerrors.Errorf("update workspace autostop: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return readBodyAsError(res)
	}
	return nil
}

type WorkspaceFilter struct {
	OrganizationID uuid.UUID
	// Owner can be a user_id (uuid), "me", or a username
	Owner string
}

// asRequestOption returns a function that can be used in (*Client).Request.
// It modifies the request query parameters.
func (f WorkspaceFilter) asRequestOption() requestOption {
	return func(r *http.Request) {
		q := r.URL.Query()
		if f.OrganizationID != uuid.Nil {
			q.Set("organization_id", f.OrganizationID.String())
		}
		if f.Owner != "" {
			q.Set("owner_id", f.Owner)
		}
		r.URL.RawQuery = q.Encode()
	}
}

// Workspaces returns all workspaces the authenticated user has access to.
func (c *Client) Workspaces(ctx context.Context, filter WorkspaceFilter) ([]Workspace, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/workspaces", nil, filter.asRequestOption())

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
