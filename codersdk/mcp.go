package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// MCPEndpoint is the remote MCP HTTP endpoint path.
const MCPEndpoint = "/api/experimental/mcp/http"

// MCPServerOAuth2ConnectURL returns the URL the user should visit to
// start the OAuth2 flow for an MCP server. The frontend opens this
// in a new window/popup.
func (c *Client) MCPServerOAuth2ConnectURL(organizationID, id uuid.UUID) string {
	return fmt.Sprintf("%s/api/v2/organizations/%s/mcp-servers/%s/oauth2/connect", c.URL.String(), organizationID, id)
}

// MCPServerOAuth2DisconnectResponse reports whether the removed token
// was also revoked at the OAuth provider.
type MCPServerOAuth2DisconnectResponse struct {
	TokenRevoked         bool   `json:"token_revoked"`
	TokenRevocationError string `json:"token_revocation_error,omitempty"`
}

// MCPServerOAuth2Disconnect removes the user's OAuth2 token for an
// MCP server. Use MCPServerOAuth2DisconnectWithResponse for the
// provider revocation outcome.
func (c *Client) MCPServerOAuth2Disconnect(ctx context.Context, id uuid.UUID) error {
	_, err := c.MCPServerOAuth2DisconnectWithResponse(ctx, id)
	return err
}

// MCPServerOAuth2DisconnectWithResponse removes the user's OAuth2
// token for an MCP server and reports the provider revocation outcome.
func (c *Client) MCPServerOAuth2DisconnectWithResponse(ctx context.Context, id uuid.UUID) (MCPServerOAuth2DisconnectResponse, error) {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/mcp/servers/%s/oauth2/disconnect", id), nil)
	if err != nil {
		return MCPServerOAuth2DisconnectResponse{}, err
	}
	defer res.Body.Close()
	// Servers from before provider revocation respond 204 without a body.
	if res.StatusCode == http.StatusNoContent {
		return MCPServerOAuth2DisconnectResponse{}, nil
	}
	if res.StatusCode != http.StatusOK {
		return MCPServerOAuth2DisconnectResponse{}, ReadBodyAsError(res)
	}
	var resp MCPServerOAuth2DisconnectResponse
	return resp, ReadBodyAsJSON(res, &resp)
}

// MCPServerConfig represents an admin-configured MCP server.
type MCPServerConfig struct {
	ID             uuid.UUID `json:"id" format:"uuid"`
	OrganizationID uuid.UUID `json:"organization_id" format:"uuid"`
	DisplayName    string    `json:"display_name"`
	Slug           string    `json:"slug"`
	Description    string    `json:"description"`
	IconURL        string    `json:"icon_url"`

	Transport string `json:"transport"` // "streamable_http" or "sse"
	URL       string `json:"url"`

	AuthType string `json:"auth_type"` // "none", "oauth2", "api_key", "custom_headers", "user_oidc", "user_api_key"

	// OAuth2 fields (only populated for admins).
	OAuth2ClientID      string `json:"oauth2_client_id,omitempty"`
	HasOAuth2Secret     bool   `json:"has_oauth2_secret"`
	OAuth2AuthURL       string `json:"oauth2_auth_url,omitempty"`
	OAuth2TokenURL      string `json:"oauth2_token_url,omitempty"`
	OAuth2RevocationURL string `json:"oauth2_revocation_url,omitempty"`
	OAuth2Resource      string `json:"oauth2_resource,omitempty"`
	OAuth2Scopes        string `json:"oauth2_scopes,omitempty"`

	// API key fields (only populated for admins, except that user_api_key
	// servers show every reader the header and prefix their token is sent
	// with).
	APIKeyHeader string `json:"api_key_header,omitempty"`
	HasAPIKey    bool   `json:"has_api_key"`
	// APIKeyPrefix is sent before each user's token for user_api_key
	// servers, such as "Bearer ".
	APIKeyPrefix string `json:"api_key_prefix,omitempty"`

	HasCustomHeaders bool `json:"has_custom_headers"`

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
	HasSigningSecret    bool      `json:"has_signing_secret"`
	CreatedAt           time.Time `json:"created_at" format:"date-time"`
	UpdatedAt           time.Time `json:"updated_at" format:"date-time"`

	// Per-user state (populated for non-admin requests). For oauth2 it
	// reports a usable token; for user_api_key it reports whether the
	// caller has stored a token.
	AuthConnected bool `json:"auth_connected"`
}

// MCPServerAuthTypeUserAPIKey is the auth type where every user stores
// their own token for the server. Chats send only their owner's token, in
// APIKeyHeader after APIKeyPrefix.
//
//nolint:gosec // An auth type name, not a credential.
const MCPServerAuthTypeUserAPIKey = "user_api_key"

// MCPServerCredentialKind is how a user, usually an agent's user,
// authenticates to a user_api_key MCP server.
type MCPServerCredentialKind string

const (
	// MCPServerCredentialKindToken is a pasted token.
	MCPServerCredentialKindToken MCPServerCredentialKind = "token"
	// MCPServerCredentialKindGitHubApp is a GitHub App installation.
	// Coder mints a short-lived installation token for every chat turn,
	// so the agent acts as the app's bot account.
	//nolint:gosec // A credential kind name, not a credential.
	MCPServerCredentialKindGitHubApp MCPServerCredentialKind = "github_app"
)

// UpdateMCPServerUserTokenRequest stores the caller's own token for a
// user_api_key MCP server. An agent's owner can instead store a GitHub App
// credential for the agent.
type UpdateMCPServerUserTokenRequest struct {
	// CredentialKind defaults to github_app when GitHubApp is set and to
	// token otherwise. github_app is only accepted for an agent.
	CredentialKind MCPServerCredentialKind `json:"credential_kind,omitempty" enums:"token,github_app"`
	// Token is required for the token kind.
	Token string `json:"token,omitempty"`
	// GitHubApp is required for the github_app kind.
	GitHubApp *MCPServerGitHubAppCredentialRequest `json:"github_app,omitempty"`
}

// MCPServerGitHubAppCredentialRequest identifies a GitHub App
// installation. Coder checks it by minting a token before saving it.
type MCPServerGitHubAppCredentialRequest struct {
	// AppID is the numeric app ID from the app's settings page.
	AppID int64 `json:"app_id"`
	// InstallationID is the number at the end of the installation's
	// settings URL.
	InstallationID int64 `json:"installation_id"`
	// PrivateKey is the app's PEM private key. It is never returned. Leave
	// it empty to keep the stored key when only other fields change.
	PrivateKey string `json:"private_key,omitempty"`
	// Repositories limits tokens to these repository names. Empty means
	// every repository the installation can access.
	Repositories []string `json:"repositories,omitempty"`
	// UseForGit also gives the app's identity to git in the agent's
	// workspaces, such as git clone and git push. Git reaches the same
	// repositories as the app's other tokens.
	UseForGit bool `json:"use_for_git,omitempty"`
}

// MCPServerGitHubApp is the public part of a stored GitHub App
// credential. The private key is never returned.
type MCPServerGitHubApp struct {
	AppID          int64    `json:"app_id"`
	InstallationID int64    `json:"installation_id"`
	Repositories   []string `json:"repositories"`
	// UseForGit is true when git in the agent's workspaces signs in as
	// the app.
	UseForGit bool `json:"use_for_git"`
}

// MCPServerUserTokenStatus reports whether a user has a token for a
// user_api_key MCP server. The token itself is never returned.
type MCPServerUserTokenStatus struct {
	HasToken  bool       `json:"has_token"`
	UpdatedAt *time.Time `json:"updated_at,omitempty" format:"date-time"`
	// CredentialKind is set when HasToken is true.
	CredentialKind MCPServerCredentialKind `json:"credential_kind,omitempty" enums:"token,github_app"`
	// GitHubApp is set for the github_app kind.
	GitHubApp *MCPServerGitHubApp `json:"github_app,omitempty"`
}

// SetMCPServerUserToken stores the caller's token for a user_api_key MCP
// server, replacing any previous one.
func (c *Client) SetMCPServerUserToken(ctx context.Context, id uuid.UUID, req UpdateMCPServerUserTokenRequest) (MCPServerUserTokenStatus, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/mcp/servers/%s/user-token", id), req)
	if err != nil {
		return MCPServerUserTokenStatus{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return MCPServerUserTokenStatus{}, ReadBodyAsError(res)
	}
	var status MCPServerUserTokenStatus
	return status, ReadBodyAsJSON(res, &status)
}

// DeleteMCPServerUserToken removes the caller's token for a user_api_key
// MCP server.
func (c *Client) DeleteMCPServerUserToken(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/mcp/servers/%s/user-token", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// MCPServerConfigRole is a role a user or group holds in an MCP server
// config's access control list.
type MCPServerConfigRole string

const (
	MCPServerConfigRoleRead MCPServerConfigRole = "read"
	// MCPServerConfigRoleDeleted removes the principal's ACL entry when
	// used in an update request.
	MCPServerConfigRoleDeleted MCPServerConfigRole = ""
)

// MCPServerConfigACL is the resolved access control list of an MCP server
// config.
type MCPServerConfigACL struct {
	Users  []MCPServerConfigUser  `json:"users"`
	Groups []MCPServerConfigGroup `json:"groups"`
}

// MCPServerConfigUser is a user entry in an MCP server config ACL.
type MCPServerConfigUser struct {
	MinimalUser
	Role MCPServerConfigRole `json:"role" enums:"read"`
}

// MCPServerConfigGroup is a group entry in an MCP server config ACL.
type MCPServerConfigGroup struct {
	Group
	Role MCPServerConfigRole `json:"role" enums:"read"`
}

// UpdateMCPServerConfigACLRequest is a sparse update of an MCP server
// config ACL: only the listed principals change, and
// MCPServerConfigRoleDeleted removes an entry.
type UpdateMCPServerConfigACLRequest struct {
	UserRoles  map[string]MCPServerConfigRole `json:"user_roles,omitempty"`
	GroupRoles map[string]MCPServerConfigRole `json:"group_roles,omitempty"`
}

// CreateMCPServerConfigRequest is the request to create a new MCP server config.
type CreateMCPServerConfigRequest struct {
	DisplayName string `json:"display_name" validate:"required"`
	Slug        string `json:"slug" validate:"required"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`

	Transport string `json:"transport" validate:"required,oneof=streamable_http sse"`
	URL       string `json:"url" validate:"required,url"`

	AuthType           string `json:"auth_type" validate:"required,oneof=none oauth2 api_key custom_headers user_oidc user_api_key"`
	OAuth2ClientID     string `json:"oauth2_client_id,omitempty"`
	OAuth2ClientSecret string `json:"oauth2_client_secret,omitempty"`
	OAuth2AuthURL      string `json:"oauth2_auth_url,omitempty" validate:"omitempty,url"`
	OAuth2TokenURL     string `json:"oauth2_token_url,omitempty" validate:"omitempty,url"`
	// OAuth2RevocationURL is the provider's RFC 7009 revocation
	// endpoint; auto-populated by OAuth2 discovery when omitted.
	OAuth2RevocationURL string `json:"oauth2_revocation_url,omitempty" validate:"omitempty,url"`
	// OAuth2Resource is the RFC 8707 resource indicator sent on OAuth2
	// requests when set. Auto-discovery fills it if empty.
	OAuth2Resource string `json:"oauth2_resource,omitempty"`
	OAuth2Scopes   string `json:"oauth2_scopes,omitempty"`
	APIKeyHeader   string `json:"api_key_header,omitempty"`
	APIKeyValue    string `json:"api_key_value,omitempty"`
	// APIKeyPrefix is sent before each user's token for user_api_key
	// servers. It defaults to "Bearer "; send "" for a bare token.
	// APIKeyHeader defaults to Authorization for user_api_key servers.
	APIKeyPrefix  *string           `json:"api_key_prefix,omitempty"`
	CustomHeaders map[string]string `json:"custom_headers,omitempty"`

	ToolAllowList []string `json:"tool_allow_list,omitempty"`
	ToolDenyList  []string `json:"tool_deny_list,omitempty"`

	Availability    string `json:"availability" validate:"required,oneof=force_on default_on default_off"`
	Enabled         bool   `json:"enabled"`
	ModelIntent     bool   `json:"model_intent"`
	AllowInPlanMode bool   `json:"allow_in_plan_mode"`

	// ForwardCoderHeaders, when true, forwards Coder identity
	// headers on every outgoing MCP request. See MCPServerConfig.
	ForwardCoderHeaders bool `json:"forward_coder_headers"`
	// SigningSecret signs forwarded identity headers and request bodies.
	// Configure the same secret on the MCP server. It is never returned.
	SigningSecret string `json:"signing_secret,omitempty"`
}

// UpdateMCPServerConfigRequest is the request to update an MCP server config.
type UpdateMCPServerConfigRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Slug        *string `json:"slug,omitempty"`
	Description *string `json:"description,omitempty"`
	IconURL     *string `json:"icon_url,omitempty"`

	Transport *string `json:"transport,omitempty" validate:"omitempty,oneof=streamable_http sse"`
	URL       *string `json:"url,omitempty" validate:"omitempty,url"`

	AuthType           *string `json:"auth_type,omitempty" validate:"omitempty,oneof=none oauth2 api_key custom_headers user_oidc user_api_key"`
	OAuth2ClientID     *string `json:"oauth2_client_id,omitempty"`
	OAuth2ClientSecret *string `json:"oauth2_client_secret,omitempty"`
	OAuth2AuthURL      *string `json:"oauth2_auth_url,omitempty" validate:"omitempty,url"`
	OAuth2TokenURL     *string `json:"oauth2_token_url,omitempty" validate:"omitempty,url"`
	// OAuth2RevocationURL is validated in the handler because a
	// validate tag would reject the pointer to "" that clears it.
	OAuth2RevocationURL *string `json:"oauth2_revocation_url,omitempty"`
	// OAuth2Resource is the RFC 8707 resource indicator. Omit it to keep
	// the stored value; send "" to clear it.
	OAuth2Resource *string `json:"oauth2_resource,omitempty"`
	OAuth2Scopes   *string `json:"oauth2_scopes,omitempty"`
	APIKeyHeader   *string `json:"api_key_header,omitempty"`
	APIKeyValue    *string `json:"api_key_value,omitempty"`
	// APIKeyPrefix: see CreateMCPServerConfigRequest.
	APIKeyPrefix  *string            `json:"api_key_prefix,omitempty"`
	CustomHeaders *map[string]string `json:"custom_headers,omitempty"`

	ToolAllowList *[]string `json:"tool_allow_list,omitempty"`
	ToolDenyList  *[]string `json:"tool_deny_list,omitempty"`

	Availability    *string `json:"availability,omitempty" validate:"omitempty,oneof=force_on default_on default_off"`
	Enabled         *bool   `json:"enabled,omitempty"`
	ModelIntent     *bool   `json:"model_intent,omitempty"`
	AllowInPlanMode *bool   `json:"allow_in_plan_mode,omitempty"`

	// ForwardCoderHeaders, when set, updates whether Coder identity
	// headers are forwarded on every outgoing MCP request.
	ForwardCoderHeaders *bool `json:"forward_coder_headers,omitempty"`
	// SigningSecret replaces the shared signing key. Omit to preserve it;
	// an empty string clears it. It is never returned.
	SigningSecret *string `json:"signing_secret,omitempty"`
}

func (c *Client) MCPServerConfigs(ctx context.Context, organizationID uuid.UUID) ([]MCPServerConfig, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/mcp-servers", organizationID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var configs []MCPServerConfig
	return configs, ReadBodyAsJSON(res, &configs)
}

func (c *Client) MCPServerConfigByID(ctx context.Context, organizationID, id uuid.UUID) (MCPServerConfig, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/mcp-servers/%s", organizationID, id), nil)
	if err != nil {
		return MCPServerConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return MCPServerConfig{}, ReadBodyAsError(res)
	}
	var config MCPServerConfig
	return config, ReadBodyAsJSON(res, &config)
}

// MCPServerConfigACL returns the resolved ACL of an MCP server config.
func (c *Client) MCPServerConfigACL(ctx context.Context, organizationID, id uuid.UUID) (MCPServerConfigACL, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/mcp-servers/%s/acl", organizationID, id), nil)
	if err != nil {
		return MCPServerConfigACL{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return MCPServerConfigACL{}, ReadBodyAsError(res)
	}
	var acl MCPServerConfigACL
	return acl, ReadBodyAsJSON(res, &acl)
}

// MCPServerConfigACLAvailable returns available users and groups that can be
// assigned to an MCP server config ACL.
func (c *Client) MCPServerConfigACLAvailable(ctx context.Context, organizationID, id uuid.UUID, req UsersRequest) (ACLAvailable, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/organizations/%s/mcp-servers/%s/acl/available", organizationID, id),
		nil,
		req.Pagination.asRequestOption(),
		req.asRequestOption(),
	)
	if err != nil {
		return ACLAvailable{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ACLAvailable{}, ReadBodyAsError(res)
	}
	var available ACLAvailable
	return available, ReadBodyAsJSON(res, &available)
}

// UpdateMCPServerConfigACL applies a sparse ACL update to an MCP server
// config.
func (c *Client) UpdateMCPServerConfigACL(ctx context.Context, organizationID, id uuid.UUID, req UpdateMCPServerConfigACLRequest) error {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s/mcp-servers/%s/acl", organizationID, id), req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

func (c *Client) CreateMCPServerConfig(ctx context.Context, organizationID uuid.UUID, req CreateMCPServerConfigRequest) (MCPServerConfig, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/mcp-servers", organizationID), req)
	if err != nil {
		return MCPServerConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return MCPServerConfig{}, ReadBodyAsError(res)
	}
	var config MCPServerConfig
	return config, ReadBodyAsJSON(res, &config)
}

func (c *Client) UpdateMCPServerConfig(ctx context.Context, organizationID, id uuid.UUID, req UpdateMCPServerConfigRequest) (MCPServerConfig, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/organizations/%s/mcp-servers/%s", organizationID, id), req)
	if err != nil {
		return MCPServerConfig{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return MCPServerConfig{}, ReadBodyAsError(res)
	}
	var config MCPServerConfig
	return config, ReadBodyAsJSON(res, &config)
}

func (c *Client) DeleteMCPServerConfig(ctx context.Context, organizationID, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/organizations/%s/mcp-servers/%s", organizationID, id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
