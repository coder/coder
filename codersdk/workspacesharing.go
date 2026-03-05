package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// WorkspaceSharingSettings represents workspace sharing settings affecting an
// organization.
type WorkspaceSharingSettings struct {
	// SharingGloballyDisabled is true if sharing has been disabled for this
	// organization because of a deployment-wide setting.
	SharingGloballyDisabled bool `json:"sharing_globally_disabled"`
	SharingDisabled         bool `json:"sharing_disabled"`
}

// UpdateWorkspaceSharingSettingsRequest represents workspace sharing settings
// that can be updated for an organization.
type UpdateWorkspaceSharingSettingsRequest struct {
	SharingDisabled bool `json:"sharing_disabled"`
}

// WorkspaceSharingSettings retrieves the workspace sharing settings for an organization.
func (c *Client) WorkspaceSharingSettings(ctx context.Context, orgID string) (WorkspaceSharingSettings, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/settings/workspace-sharing", orgID), nil)
	if err != nil {
		return WorkspaceSharingSettings{}, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return WorkspaceSharingSettings{}, ReadBodyAsError(res)
	}
	var resp WorkspaceSharingSettings
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// PatchWorkspaceSharingSettings modifies the workspace sharing settings for an organization.
func (c *Client) PatchWorkspaceSharingSettings(ctx context.Context, orgID string, req UpdateWorkspaceSharingSettingsRequest) (WorkspaceSharingSettings, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s/settings/workspace-sharing", orgID), req)
	if err != nil {
		return WorkspaceSharingSettings{}, err
	}

	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return WorkspaceSharingSettings{}, ReadBodyAsError(res)
	}
	var resp WorkspaceSharingSettings
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
