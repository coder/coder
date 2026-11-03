package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/xerrors"
)

// GroupSyncWarning reports one finding in group sync settings that the server
// accepted, and what it did about it. Warnings accompany the settings in API
// responses, so that a client can compare what it sent with what was saved.
type GroupSyncWarning struct {
	// Code names the condition violated.
	Code GroupSyncFindingCode `json:"code"`
	// Path is a JSON Pointer (RFC 6901) to the element concerned, in the
	// settings as received. The empty path refers to the whole text.
	Path string `json:"path"`
	// Received is the JSON text of the element at Path, as received.
	Received json.RawMessage `json:"received"`
	// Accepted is the JSON text of the element in the settings as saved or
	// returned. It is absent when the element was removed, and equal to
	// Received when the settings were not rewritten.
	Accepted json.RawMessage `json:"accepted,omitempty"`
}

// GroupSyncSettingsResponse is what the group sync settings endpoints
// return: the settings as saved or returned, and a warning for each finding
// the server accepted. The settings are embedded, so the response has the
// same members as GroupSyncSettings plus `warnings` and `received_settings`,
// and a client that decodes it as GroupSyncSettings reads the settings
// unchanged.
type GroupSyncSettingsResponse struct {
	GroupSyncSettings
	// Warnings holds one warning for each finding the server accepted. Their
	// paths point into ReceivedSettings.
	Warnings []GroupSyncWarning `json:"warnings"`
	// ReceivedSettings is the settings the warnings refer to: the request for
	// the full-replace endpoint, the merged settings before rewriting for the
	// config and mapping-diff endpoints, and the stored settings as read for
	// GET. It is present exactly when there are warnings.
	ReceivedSettings *GroupSyncSettings `json:"received_settings,omitempty"`
}

// The WithWarnings methods return the warnings along with the settings. They
// will become the ordinary group sync settings methods over time; the
// methods without the suffix discard the warnings and are deprecated.

// GroupIDPSyncSettingsWithWarnings returns an organization's group sync
// settings and the warnings for them.
func (c *Client) GroupIDPSyncSettingsWithWarnings(ctx context.Context, orgID string) (GroupSyncSettingsResponse, error) {
	return c.groupSyncSettingsRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/groups", orgID), nil)
}

// PatchGroupIDPSyncSettingsWithWarnings replaces an organization's group sync
// settings, and returns them as saved with the warnings for them.
func (c *Client) PatchGroupIDPSyncSettingsWithWarnings(ctx context.Context, orgID string, req GroupSyncSettings) (GroupSyncSettingsResponse, error) {
	return c.groupSyncSettingsRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/groups", orgID), req)
}

// PatchGroupIDPSyncConfigWithWarnings changes the configuration members of an
// organization's group sync settings, and returns the settings as saved with
// the warnings for them.
func (c *Client) PatchGroupIDPSyncConfigWithWarnings(ctx context.Context, orgID string, req PatchGroupIDPSyncConfigRequest) (GroupSyncSettingsResponse, error) {
	return c.groupSyncSettingsRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/groups/config", orgID), req)
}

// PatchGroupIDPSyncMappingWithWarnings adds and removes entries in an
// organization's group sync mapping, and returns the settings as saved with
// the warnings for them.
func (c *Client) PatchGroupIDPSyncMappingWithWarnings(ctx context.Context, orgID string, req PatchGroupIDPSyncMappingRequest) (GroupSyncSettingsResponse, error) {
	return c.groupSyncSettingsRequest(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/groups/mapping", orgID), req)
}

// groupSyncSettingsRequest makes a request to a group sync settings endpoint
// and decodes the response.
func (c *Client) groupSyncSettingsRequest(ctx context.Context, method, path string, body any) (GroupSyncSettingsResponse, error) {
	res, err := c.Request(ctx, method, path, body)
	if err != nil {
		return GroupSyncSettingsResponse{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GroupSyncSettingsResponse{}, ReadBodyAsError(res)
	}
	var resp GroupSyncSettingsResponse
	return resp, ReadBodyAsJSON(res, &resp)
}
