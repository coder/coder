package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type GroupSyncSettings struct {
	// Field selects the claim field to be used as the created user's
	// groups. If the group field is the empty string, then no group updates
	// will ever come from the OIDC provider.
	Field string `json:"field"`
	// Mapping maps from an OIDC group --> Coder group ID
	Mapping map[string][]uuid.UUID `json:"mapping"`
	// RegexFilter is a regular expression that filters the groups returned by
	// the OIDC provider. Any group not matched by this regex will be ignored.
	// If the group filter is nil, then no group filtering will occur.
	RegexFilter *regexp.Regexp `json:"regex_filter"`
	// AutoCreateMissing controls whether groups returned by the OIDC provider
	// are automatically created in Coder if they are missing.
	AutoCreateMissing bool `json:"auto_create_missing_groups"`
	// LegacyNameMapping is deprecated. It remaps an IDP group name to
	// a Coder group name. Since configuration is now done at runtime,
	// group IDs are used to account for group renames.
	// For legacy configurations, this config option has to remain.
	// Deprecated: Use Mapping instead.
	LegacyNameMapping map[string]string `json:"legacy_group_name_mapping,omitempty"`
}

func (c *Client) GroupIDPSyncSettings(ctx context.Context, orgID string) (GroupSyncSettings, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/groups", orgID), nil)
	if err != nil {
		return GroupSyncSettings{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GroupSyncSettings{}, ReadBodyAsError(res)
	}
	var resp GroupSyncSettings
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) PatchGroupIDPSyncSettings(ctx context.Context, orgID string, req GroupSyncSettings) (GroupSyncSettings, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/groups", orgID), req)
	if err != nil {
		return GroupSyncSettings{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GroupSyncSettings{}, ReadBodyAsError(res)
	}
	var resp GroupSyncSettings
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

type RoleSyncSettings struct {
	// Field selects the claim field to be used as the created user's
	// groups. If the group field is the empty string, then no group updates
	// will ever come from the OIDC provider.
	Field string `json:"field"`
	// Mapping maps from an OIDC group --> Coder organization role
	Mapping map[string][]string `json:"mapping"`
}

func (c *Client) RoleIDPSyncSettings(ctx context.Context, orgID string) (RoleSyncSettings, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/roles", orgID), nil)
	if err != nil {
		return RoleSyncSettings{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return RoleSyncSettings{}, ReadBodyAsError(res)
	}
	var resp RoleSyncSettings
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) PatchRoleIDPSyncSettings(ctx context.Context, orgID string, req RoleSyncSettings) (RoleSyncSettings, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/roles", orgID), req)
	if err != nil {
		return RoleSyncSettings{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return RoleSyncSettings{}, ReadBodyAsError(res)
	}
	var resp RoleSyncSettings
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
