package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// MCPServerOAuth2ConnectURL returns the URL the user should visit to
// start the OAuth2 flow for an MCP server. The frontend opens this
// in a new window/popup.
func (c *Client) MCPServerOAuth2ConnectURL(id uuid.UUID) string {
	return fmt.Sprintf("%s/api/experimental/mcp/servers/%s/oauth2/connect", c.URL.String(), id)
}

// MCPServerOAuth2Disconnect removes the user's OAuth2 token for an
// MCP server.
func (c *Client) MCPServerOAuth2Disconnect(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/experimental/mcp/servers/%s/oauth2/disconnect", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// MCPServerConfig represents an admin-configured MCP server.
type MCPServerConfig struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	DisplayName string    `json:"display_name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	IconURL     string    `json:"icon_url"`

	Transport string `json:"transport"` // "streamable_http" or "sse"
	URL       string `json:"url"`

	AuthType string `json:"auth_type"` // "none", "oauth2", "api_key", "custom_headers", "user_oidc"

	// OAuth2 fields (only populated for admins).
	OAuth2ClientID  string `json:"oauth2_client_id,omitempty"`
	HasOAuth2Secret bool   `json:"has_oauth2_secret"`
	OAuth2AuthURL   string `json:"oauth2_auth_url,omitempty"`
	OAuth2TokenURL  string `json:"oauth2_token_url,omitempty"`
	OAuth2Scopes    string `json:"oauth2_scopes,omitempty"`

	// API key fields (only populated for admins).
	APIKeyHeader string `json:"api_key_header,omitempty"`
	HasAPIKey    bool   `json:"has_api_key"`

	HasCustomHeaders bool `json:"has_custom_headers"`

	// CustomHeadersUserKeys lists custom_headers entries whose values
	// are supplied per-user. These are visible to all callers so the
	// user settings UI can prompt for the corresponding values. The
	// set must be disjoint from the admin-set CustomHeaders keys.
	CustomHeadersUserKeys []string `json:"custom_headers_user_keys"`

	// CustomHeadersUserKeyDescriptions maps a user-set custom header
	// name to optional helper text the admin wrote to explain what
	// the user should enter. Keys are case-insensitively a subset of
	// CustomHeadersUserKeys. Missing entries mean "no description".
	CustomHeadersUserKeyDescriptions map[string]string `json:"custom_headers_user_key_descriptions"`

	// Tool governance.
	ToolAllowList []string `json:"tool_allow_list"`
	ToolDenyList  []string `json:"tool_deny_list"`

	// Availability policy set by admin.
	Availability string `json:"availability"` // "force_on", "default_on", "default_off"

	Enabled         bool `json:"enabled"`
	ModelIntent     bool `json:"model_intent"`
	AllowInPlanMode bool `json:"allow_in_plan_mode"`

	// ForwardCoderHeaders forwards the same Coder identity headers we
	// send to LLM providers (X-Coder-Owner-Id, X-Coder-Chat-Id, and the
	// optional X-Coder-Subchat-Id and X-Coder-Workspace-Id) to this
	// MCP server on every request. Off by default to avoid leaking
	// chat identity to third-party servers.
	ForwardCoderHeaders bool      `json:"forward_coder_headers"`
	CreatedAt           time.Time `json:"created_at" format:"date-time"`
	UpdatedAt           time.Time `json:"updated_at" format:"date-time"`

	// Per-user state (populated for non-admin requests).
	AuthConnected bool `json:"auth_connected"`
}

// CreateMCPServerConfigRequest is the request to create a new MCP server config.
type CreateMCPServerConfigRequest struct {
	DisplayName string `json:"display_name" validate:"required"`
	Slug        string `json:"slug" validate:"required"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`

	Transport string `json:"transport" validate:"required,oneof=streamable_http sse"`
	URL       string `json:"url" validate:"required,url"`

	AuthType           string            `json:"auth_type" validate:"required,oneof=none oauth2 api_key custom_headers user_oidc"`
	OAuth2ClientID     string            `json:"oauth2_client_id,omitempty"`
	OAuth2ClientSecret string            `json:"oauth2_client_secret,omitempty"`
	OAuth2AuthURL      string            `json:"oauth2_auth_url,omitempty" validate:"omitempty,url"`
	OAuth2TokenURL     string            `json:"oauth2_token_url,omitempty" validate:"omitempty,url"`
	OAuth2Scopes       string            `json:"oauth2_scopes,omitempty"`
	APIKeyHeader       string            `json:"api_key_header,omitempty"`
	APIKeyValue        string            `json:"api_key_value,omitempty"`
	CustomHeaders      map[string]string `json:"custom_headers,omitempty"`

	// CustomHeadersUserKeys, when AuthType is "custom_headers", marks
	// these header names as user-supplied. Each entry must be disjoint
	// from CustomHeaders keys and from each other (case-insensitive).
	CustomHeadersUserKeys []string `json:"custom_headers_user_keys,omitempty"`

	// CustomHeadersUserKeyDescriptions optionally provides helper text
	// per user-set header key. Keys must be a (case-insensitive) subset
	// of CustomHeadersUserKeys; descriptions for unknown keys are
	// rejected. Empty strings are dropped.
	CustomHeadersUserKeyDescriptions map[string]string `json:"custom_headers_user_key_descriptions,omitempty"`

	ToolAllowList []string `json:"tool_allow_list,omitempty"`
	ToolDenyList  []string `json:"tool_deny_list,omitempty"`

	Availability    string `json:"availability" validate:"required,oneof=force_on default_on default_off"`
	Enabled         bool   `json:"enabled"`
	ModelIntent     bool   `json:"model_intent"`
	AllowInPlanMode bool   `json:"allow_in_plan_mode"`

	// ForwardCoderHeaders, when true, forwards Coder identity
	// headers on every outgoing MCP request. See MCPServerConfig.
	ForwardCoderHeaders bool `json:"forward_coder_headers"`
}

// UpdateMCPServerConfigRequest is the request to update an MCP server config.
type UpdateMCPServerConfigRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description,omitempty"`
	IconURL     *string `json:"icon_url,omitempty"`

	Transport *string `json:"transport,omitempty" validate:"omitempty,oneof=streamable_http sse"`
	URL       *string `json:"url,omitempty" validate:"omitempty,url"`

	AuthType           *string            `json:"auth_type,omitempty" validate:"omitempty,oneof=none oauth2 api_key custom_headers user_oidc"`
	OAuth2ClientID     *string            `json:"oauth2_client_id,omitempty"`
	OAuth2ClientSecret *string            `json:"oauth2_client_secret,omitempty"`
	OAuth2AuthURL      *string            `json:"oauth2_auth_url,omitempty" validate:"omitempty,url"`
	OAuth2TokenURL     *string            `json:"oauth2_token_url,omitempty" validate:"omitempty,url"`
	OAuth2Scopes       *string            `json:"oauth2_scopes,omitempty"`
	APIKeyHeader       *string            `json:"api_key_header,omitempty"`
	APIKeyValue        *string            `json:"api_key_value,omitempty"`
	CustomHeaders      *map[string]string `json:"custom_headers,omitempty"`

	// CustomHeadersUserKeys, when non-nil, replaces the set of
	// user-supplied header names. See MCPServerConfig.CustomHeadersUserKeys.
	CustomHeadersUserKeys *[]string `json:"custom_headers_user_keys,omitempty"`

	// CustomHeadersUserKeyDescriptions, when non-nil, replaces the
	// per-key description map. Pass an empty map to clear all
	// descriptions. See MCPServerConfig.CustomHeadersUserKeyDescriptions.
	CustomHeadersUserKeyDescriptions *map[string]string `json:"custom_headers_user_key_descriptions,omitempty"`

	ToolAllowList *[]string `json:"tool_allow_list,omitempty"`
	ToolDenyList  *[]string `json:"tool_deny_list,omitempty"`

	Availability    *string `json:"availability,omitempty" validate:"omitempty,oneof=force_on default_on default_off"`
	Enabled         *bool   `json:"enabled,omitempty"`
	ModelIntent     *bool   `json:"model_intent,omitempty"`
	AllowInPlanMode *bool   `json:"allow_in_plan_mode,omitempty"`

	// ForwardCoderHeaders, when set, updates whether Coder identity
	// headers are forwarded on every outgoing MCP request.
	ForwardCoderHeaders *bool `json:"forward_coder_headers,omitempty"`
}

// MCPServerUserHeaderValues represents the calling user's state for
// an MCP server with admin-marked user-set custom headers. Values are
// never returned; HasValues records which keys currently have a
// non-empty user-supplied value.
type MCPServerUserHeaderValues struct {
	MCPServerConfigID uuid.UUID       `json:"mcp_server_config_id" format:"uuid"`
	HasValues         map[string]bool `json:"has_values"`
}

// UpdateMCPServerUserHeaderValuesRequest upserts the calling user's
// values for an MCP server's user-set custom headers. The set of keys
// in Values must be a subset of the server's CustomHeadersUserKeys;
// any keys not present are left unchanged. To clear a single value,
// pass an empty string; to clear all values, use DeleteMCPServerUserHeaderValues.
type UpdateMCPServerUserHeaderValuesRequest struct {
	Values map[string]string `json:"values" validate:"required"`
}

func (c *Client) MCPServerConfigs(ctx context.Context) ([]MCPServerConfig, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/mcp/servers", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var configs []MCPServerConfig
	return configs, json.NewDecoder(res.Body).Decode(&configs)
}

func (c *Client) MCPServerConfigByID(ctx context.Context, id uuid.UUID) (MCPServerConfig, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/mcp/servers/%s", id), nil)
	if err != nil {
		return MCPServerConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return MCPServerConfig{}, ReadBodyAsError(res)
	}
	var config MCPServerConfig
	return config, json.NewDecoder(res.Body).Decode(&config)
}

func (c *Client) CreateMCPServerConfig(ctx context.Context, req CreateMCPServerConfigRequest) (MCPServerConfig, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/experimental/mcp/servers", req)
	if err != nil {
		return MCPServerConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return MCPServerConfig{}, ReadBodyAsError(res)
	}
	var config MCPServerConfig
	return config, json.NewDecoder(res.Body).Decode(&config)
}

func (c *Client) UpdateMCPServerConfig(ctx context.Context, id uuid.UUID, req UpdateMCPServerConfigRequest) (MCPServerConfig, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/experimental/mcp/servers/%s", id), req)
	if err != nil {
		return MCPServerConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return MCPServerConfig{}, ReadBodyAsError(res)
	}
	var config MCPServerConfig
	return config, json.NewDecoder(res.Body).Decode(&config)
}

func (c *Client) DeleteMCPServerConfig(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/experimental/mcp/servers/%s", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// MCPServerUserHeaderValues returns the calling user's HasValues map
// for the given MCP server. Returns an empty HasValues map when no
// user-set values have been stored yet.
func (c *Client) MCPServerUserHeaderValues(ctx context.Context, id uuid.UUID) (MCPServerUserHeaderValues, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/mcp/servers/%s/user-headers", id), nil)
	if err != nil {
		return MCPServerUserHeaderValues{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return MCPServerUserHeaderValues{}, ReadBodyAsError(res)
	}
	var resp MCPServerUserHeaderValues
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// UpdateMCPServerUserHeaderValues upserts the calling user's values
// for the given MCP server's user-set custom headers.
func (c *Client) UpdateMCPServerUserHeaderValues(ctx context.Context, id uuid.UUID, req UpdateMCPServerUserHeaderValuesRequest) (MCPServerUserHeaderValues, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/experimental/mcp/servers/%s/user-headers", id), req)
	if err != nil {
		return MCPServerUserHeaderValues{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return MCPServerUserHeaderValues{}, ReadBodyAsError(res)
	}
	var resp MCPServerUserHeaderValues
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DeleteMCPServerUserHeaderValues removes the calling user's values
// for the given MCP server's user-set custom headers.
func (c *Client) DeleteMCPServerUserHeaderValues(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/experimental/mcp/servers/%s/user-headers", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
