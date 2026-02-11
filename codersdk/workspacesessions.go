package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// WorkspaceSessionsResponse is the response for listing workspace sessions.
type WorkspaceSessionsResponse struct {
	Sessions []WorkspaceSession `json:"sessions"`
	Count    int64              `json:"count"`
}

// WorkspaceSessions returns the sessions for a workspace.
func (c *Client) WorkspaceSessions(ctx context.Context, workspaceID uuid.UUID) (WorkspaceSessionsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspaces/%s/sessions", workspaceID), nil)
	if err != nil {
		return WorkspaceSessionsResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceSessionsResponse{}, ReadBodyAsError(res)
	}
	var resp WorkspaceSessionsResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// GlobalWorkspaceSession extends WorkspaceSession with workspace
// metadata for the global sessions view.
type GlobalWorkspaceSession struct {
	WorkspaceSession
	WorkspaceID            uuid.UUID `json:"workspace_id" format:"uuid"`
	WorkspaceName          string    `json:"workspace_name"`
	WorkspaceOwnerUsername string    `json:"workspace_owner_username"`
}

// GlobalWorkspaceSessionsResponse is the response for the global
// workspace sessions endpoint.
type GlobalWorkspaceSessionsResponse struct {
	Sessions []GlobalWorkspaceSession `json:"sessions"`
	Count    int64                    `json:"count"`
}

// GlobalWorkspaceSessionsRequest is the request for the global
// workspace sessions endpoint.
type GlobalWorkspaceSessionsRequest struct {
	SearchQuery string `json:"q,omitempty"`
	Pagination
}

// GlobalWorkspaceSessions returns workspace sessions across all
// workspaces, with optional search filters.
func (c *Client) GlobalWorkspaceSessions(ctx context.Context, req GlobalWorkspaceSessionsRequest) (GlobalWorkspaceSessionsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/connectionlog/sessions", nil, req.Pagination.asRequestOption(), func(r *http.Request) {
		q := r.URL.Query()
		var params []string
		if req.SearchQuery != "" {
			params = append(params, req.SearchQuery)
		}
		q.Set("q", strings.Join(params, " "))
		r.URL.RawQuery = q.Encode()
	})
	if err != nil {
		return GlobalWorkspaceSessionsResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GlobalWorkspaceSessionsResponse{}, ReadBodyAsError(res)
	}

	var resp GlobalWorkspaceSessionsResponse
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return GlobalWorkspaceSessionsResponse{}, err
	}
	return resp, nil
}
