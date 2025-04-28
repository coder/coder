package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type IDPSyncMapping[ResourceIdType uuid.UUID | string] struct {
	// The IdP claim the user has
	Given string
	// The ID of the Coder resource the user should be added to
	Gets ResourceIdType
}

type GroupSyncSettings struct {
	// Field is the name of the claim field that specifies what groups a user
	// should be in. If empty, no groups will be synced.
	Field string `json:"field"`
	// Mapping is a map from OIDC groups to Coder group IDs
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

type PatchGroupIDPSyncConfigRequest struct {
	Field             string         `json:"field"`
	RegexFilter       *regexp.Regexp `json:"regex_filter"`
	AutoCreateMissing bool           `json:"auto_create_missing_groups"`
}

func (c *Client) PatchGroupIDPSyncConfig(ctx context.Context, orgID string, req PatchGroupIDPSyncConfigRequest) (GroupSyncSettings, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/groups/config", orgID), req)
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

// If the same mapping is present in both Add and Remove, Remove will take presidence.
type PatchGroupIDPSyncMappingRequest struct {
	Add    []IDPSyncMapping[uuid.UUID]
	Remove []IDPSyncMapping[uuid.UUID]
}

func (c *Client) PatchGroupIDPSyncMapping(ctx context.Context, orgID string, req PatchGroupIDPSyncMappingRequest) (GroupSyncSettings, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/groups/mapping", orgID), req)
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
	// Field is the name of the claim field that specifies what organization roles
	// a user should be given. If empty, no roles will be synced.
	Field string `json:"field"`
	// Mapping is a map from OIDC groups to Coder organization roles.
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

type PatchRoleIDPSyncConfigRequest struct {
	Field string `json:"field"`
}

func (c *Client) PatchRoleIDPSyncConfig(ctx context.Context, orgID string, req PatchRoleIDPSyncConfigRequest) (RoleSyncSettings, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/roles/config", orgID), req)
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

// If the same mapping is present in both Add and Remove, Remove will take presidence.
type PatchRoleIDPSyncMappingRequest struct {
	Add    []IDPSyncMapping[string]
	Remove []IDPSyncMapping[string]
}

func (c *Client) PatchRoleIDPSyncMapping(ctx context.Context, orgID string, req PatchRoleIDPSyncMappingRequest) (RoleSyncSettings, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/roles/mapping", orgID), req)
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

type OrganizationSyncSettings struct {
	// Field selects the claim field to be used as the created user's
	// organizations. If the field is the empty string, then no organization
	// updates will ever come from the OIDC provider.
	Field string `json:"field"`
	// Mapping maps from an OIDC claim --> Coder organization uuid
	Mapping map[string][]uuid.UUID `json:"mapping"`
	// AssignDefault will ensure the default org is always included
	// for every user, regardless of their claims. This preserves legacy behavior.
	AssignDefault bool `json:"organization_assign_default"`
}

func (c *Client) OrganizationIDPSyncSettings(ctx context.Context) (OrganizationSyncSettings, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/settings/idpsync/organization", nil)
	if err != nil {
		return OrganizationSyncSettings{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return OrganizationSyncSettings{}, ReadBodyAsError(res)
	}
	var resp OrganizationSyncSettings
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) PatchOrganizationIDPSyncSettings(ctx context.Context, req OrganizationSyncSettings) (OrganizationSyncSettings, error) {
	res, err := c.Request(ctx, http.MethodPatch, "/api/v2/settings/idpsync/organization", req)
	if err != nil {
		return OrganizationSyncSettings{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return OrganizationSyncSettings{}, ReadBodyAsError(res)
	}
	var resp OrganizationSyncSettings
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

type PatchOrganizationIDPSyncConfigRequest struct {
	Field         string `json:"field"`
	AssignDefault bool   `json:"assign_default"`
}

func (c *Client) PatchOrganizationIDPSyncConfig(ctx context.Context, req PatchOrganizationIDPSyncConfigRequest) (OrganizationSyncSettings, error) {
	res, err := c.Request(ctx, http.MethodPatch, "/api/v2/settings/idpsync/organization/config", req)
	if err != nil {
		return OrganizationSyncSettings{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return OrganizationSyncSettings{}, ReadBodyAsError(res)
	}
	var resp OrganizationSyncSettings
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// If the same mapping is present in both Add and Remove, Remove will take presidence.
type PatchOrganizationIDPSyncMappingRequest struct {
	Add    []IDPSyncMapping[uuid.UUID]
	Remove []IDPSyncMapping[uuid.UUID]
}

func (c *Client) PatchOrganizationIDPSyncMapping(ctx context.Context, req PatchOrganizationIDPSyncMappingRequest) (OrganizationSyncSettings, error) {
	res, err := c.Request(ctx, http.MethodPatch, "/api/v2/settings/idpsync/organization/mapping", req)
	if err != nil {
		return OrganizationSyncSettings{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return OrganizationSyncSettings{}, ReadBodyAsError(res)
	}
	var resp OrganizationSyncSettings
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) GetAvailableIDPSyncFields(ctx context.Context) ([]string, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/settings/idpsync/available-fields", nil)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var resp []string
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) GetOrganizationAvailableIDPSyncFields(ctx context.Context, orgID string) ([]string, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/available-fields", orgID), nil)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var resp []string
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) GetIDPSyncFieldValues(ctx context.Context, claimField string) ([]string, error) {
	qv := url.Values{}
	qv.Add("claimField", claimField)
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/settings/idpsync/field-values?%s", qv.Encode()), nil)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var resp []string
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

func (c *Client) GetOrganizationIDPSyncFieldValues(ctx context.Context, orgID string, claimField string) ([]string, error) {
	qv := url.Values{}
	qv.Add("claimField", claimField)
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/settings/idpsync/field-values?%s", orgID, qv.Encode()), nil)
	if err != nil {
		return nil, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var resp []string
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
