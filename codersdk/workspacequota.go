package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type WorkspaceQuota struct {
	UserWorkspaceCount int `json:"user_workspace_count"`
	UserWorkspaceLimit int `json:"user_workspace_limit"`
}

func (c *Client) WorkspaceQuota(ctx context.Context, userID string) (WorkspaceQuota, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/workspace-quota/%s", userID), nil)
	if err != nil {
		return WorkspaceQuota{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceQuota{}, readBodyAsError(res)
	}
	var quota WorkspaceQuota
	return quota, json.NewDecoder(res.Body).Decode(&quota)
}
