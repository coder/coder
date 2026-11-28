package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/xerrors"
)

// GroupSyncFindingCode names a condition that group sync settings violate.
// The codes are stable: they appear in API responses and may later be stored
// as part of a policy.
//
// All the codes are declared in this file. The API documentation generator
// orders an enum's values by the order in which it reads files, which is not
// fixed, so codes split across files would make the generated documentation
// vary from run to run.
type GroupSyncFindingCode string

// Finding codes reported by the syntax validator. Each names a violation of
// one of the well-formedness conditions W1 through W8 in
// `coderd/idpsync/SYNC_SETTINGS.md`.
const (
	// GroupSyncFindingFieldEmpty: `field` is empty while another member is
	// nonempty (W4).
	GroupSyncFindingFieldEmpty GroupSyncFindingCode = "field_empty"
	// GroupSyncFindingEmptyMappingKey: a member name of `mapping` is the
	// empty string (W5).
	GroupSyncFindingEmptyMappingKey GroupSyncFindingCode = "empty_mapping_key"
	// GroupSyncFindingNullArray: a member of `mapping` has the value `null`
	// (W5).
	GroupSyncFindingNullArray GroupSyncFindingCode = "null_array"
	// GroupSyncFindingNilUUID: a `mapping` array contains the nil UUID (W5).
	GroupSyncFindingNilUUID GroupSyncFindingCode = "nil_uuid"
	// GroupSyncFindingRepeatedUUID: a `mapping` array contains a UUID that
	// already appears earlier in the same array (W5). The nil UUID is
	// reported as GroupSyncFindingNilUUID at every position, never as a
	// repeat.
	GroupSyncFindingRepeatedUUID GroupSyncFindingCode = "repeated_uuid"
	// GroupSyncFindingFilterExcludedKey: `regex_filter` is a nonempty string
	// that does not match a member name of `mapping` (W6).
	GroupSyncFindingFilterExcludedKey GroupSyncFindingCode = "filter_excluded_key"
	// GroupSyncFindingLegacyMapping: `legacy_group_name_mapping` is nonempty
	// (W8).
	GroupSyncFindingLegacyMapping GroupSyncFindingCode = "legacy_mapping"
)

// Finding codes for the validity conditions V2 through V5 in
// `coderd/idpsync/SYNC_SETTINGS.md`, in addition to the syntax codes. Validity
// depends on the state of the deployment, so only the server reports these
// codes, through ValidityFindings in `coderd/idpsync`. They appear in API
// warnings and rejections alongside the syntax codes.
const (
	// GroupSyncFindingOrganizationDeleted: the target organization is
	// soft-deleted (V2).
	GroupSyncFindingOrganizationDeleted GroupSyncFindingCode = "organization_deleted"
	// GroupSyncFindingOrganizationAbsent: no organization has the target ID
	// (V2).
	GroupSyncFindingOrganizationAbsent GroupSyncFindingCode = "organization_absent"
	// GroupSyncFindingMissingGroup: a `mapping` array contains a UUID that
	// names no group (V3).
	GroupSyncFindingMissingGroup GroupSyncFindingCode = "missing_group"
	// GroupSyncFindingOutsideGroup: a `mapping` array contains a UUID that
	// names a group in another organization (V4).
	GroupSyncFindingOutsideGroup GroupSyncFindingCode = "outside_group"
	// GroupSyncFindingEveryoneGroup: a `mapping` array contains the target
	// organization's Everyone group (V5).
	GroupSyncFindingEveryoneGroup GroupSyncFindingCode = "everyone_group"
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
	// Received is the JSON text of the element at Path, as received. Its
	// kind follows from the element Path names: a UUID string for an element
	// of a mapping array, an array or null for a mapping value, the member's
	// value for a top-level member, and the whole settings object for the
	// empty path.
	Received json.RawMessage `json:"received" swaggertype:"object"`
	// Accepted is the JSON text of the element in the settings as saved or
	// returned, of the same kind as Received. It is absent when the element
	// was removed, and equal to Received when the settings were not
	// rewritten.
	Accepted json.RawMessage `json:"accepted,omitempty" swaggertype:"object"`
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
