package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

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
