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

	AuthType string `json:"auth_type"` // "none", "oauth2", "api_key", "custom_headers"

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

	// Tool governance.
	ToolAllowList []string `json:"tool_allow_list"`
	ToolDenyList  []string `json:"tool_deny_list"`

	// Availability policy set by admin.
	Availability string `json:"availability"` // "force_on", "default_on", "default_off"

	Enabled     bool      `json:"enabled"`
	ModelIntent bool      `json:"model_intent"`
	CreatedAt   time.Time `json:"created_at" format:"date-time"`
	UpdatedAt   time.Time `json:"updated_at" format:"date-time"`

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

	AuthType           string            `json:"auth_type" validate:"required,oneof=none oauth2 api_key custom_headers"`
	OAuth2ClientID     string            `json:"oauth2_client_id,omitempty"`
	OAuth2ClientSecret string            `json:"oauth2_client_secret,omitempty"`
	OAuth2AuthURL      string            `json:"oauth2_auth_url,omitempty" validate:"omitempty,url"`
	OAuth2TokenURL     string            `json:"oauth2_token_url,omitempty" validate:"omitempty,url"`
	OAuth2Scopes       string            `json:"oauth2_scopes,omitempty"`
	APIKeyHeader       string            `json:"api_key_header,omitempty"`
	APIKeyValue        string            `json:"api_key_value,omitempty"`
	CustomHeaders      map[string]string `json:"custom_headers,omitempty"`

	ToolAllowList []string `json:"tool_allow_list,omitempty"`
	ToolDenyList  []string `json:"tool_deny_list,omitempty"`

	Availability string `json:"availability" validate:"required,oneof=force_on default_on default_off"`
	Enabled      bool   `json:"enabled"`
	ModelIntent  bool   `json:"model_intent"`
}

// UpdateMCPServerConfigRequest is the request to update an MCP server config.
type UpdateMCPServerConfigRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description,omitempty"`
	IconURL     *string `json:"icon_url,omitempty"`

	Transport *string `json:"transport,omitempty" validate:"omitempty,oneof=streamable_http sse"`
	URL       *string `json:"url,omitempty" validate:"omitempty,url"`

	AuthType           *string            `json:"auth_type,omitempty" validate:"omitempty,oneof=none oauth2 api_key custom_headers"`
	OAuth2ClientID     *string            `json:"oauth2_client_id,omitempty"`
	OAuth2ClientSecret *string            `json:"oauth2_client_secret,omitempty"`
	OAuth2AuthURL      *string            `json:"oauth2_auth_url,omitempty" validate:"omitempty,url"`
	OAuth2TokenURL     *string            `json:"oauth2_token_url,omitempty" validate:"omitempty,url"`
	OAuth2Scopes       *string            `json:"oauth2_scopes,omitempty"`
	APIKeyHeader       *string            `json:"api_key_header,omitempty"`
	APIKeyValue        *string            `json:"api_key_value,omitempty"`
	CustomHeaders      *map[string]string `json:"custom_headers,omitempty"`

	ToolAllowList *[]string `json:"tool_allow_list,omitempty"`
	ToolDenyList  *[]string `json:"tool_deny_list,omitempty"`

	Availability *string `json:"availability,omitempty" validate:"omitempty,oneof=force_on default_on default_off"`
	Enabled      *bool   `json:"enabled,omitempty"`
	ModelIntent  *bool   `json:"model_intent,omitempty"`
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
