package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ShareableWorkspaceOwners controls whose workspaces can be shared
// within an organization.
type ShareableWorkspaceOwners string

const (
	ShareableWorkspaceOwnersNone            ShareableWorkspaceOwners = "none"
	ShareableWorkspaceOwnersEveryone        ShareableWorkspaceOwners = "everyone"
	ShareableWorkspaceOwnersServiceAccounts ShareableWorkspaceOwners = "service_accounts"
)

// WorkspaceSharingSettings represents workspace sharing settings affecting an
// organization.
type WorkspaceSharingSettings struct {
	// SharingGloballyDisabled is true if sharing has been disabled for this
	// organization because of a deployment-wide setting.
	SharingGloballyDisabled bool `json:"sharing_globally_disabled"`
	// SharingDisabled is deprecated and left for backward compatibility
	// purposes.
	// Deprecated: use `ShareableWorkspaceOwners` instead
	SharingDisabled bool `json:"sharing_disabled"`
	// ShareableWorkspaceOwners controls whose workspaces can be shared
	// within the organization.
	ShareableWorkspaceOwners ShareableWorkspaceOwners `json:"shareable_workspace_owners" enums:"none,everyone,service_accounts"`
}

// UpdateWorkspaceSharingSettingsRequest represents workspace sharing settings
// that can be updated for an organization.
type UpdateWorkspaceSharingSettingsRequest struct {
	// SharingDisabled is deprecated and left for backward compatibility
	// purposes.
	// Deprecated: use `ShareableWorkspaceOwners` instead
	SharingDisabled bool `json:"sharing_disabled"`
	// ShareableWorkspaceOwners controls whose workspaces can be shared
	// within the organization.
	ShareableWorkspaceOwners ShareableWorkspaceOwners `json:"shareable_workspace_owners,omitempty" enums:"none,everyone,service_accounts"`
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
