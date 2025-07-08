package codersdk

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/xerrors"
)

const (
	oauth2DeviceActionAuthorize = "authorize"
	oauth2DeviceActionDeny      = "deny"
)

type OAuth2ProviderApp struct {
	ID           uuid.UUID `json:"id" format:"uuid"`
	Name         string    `json:"name"`
	RedirectURIs []string  `json:"redirect_uris"`
	Icon         string    `json:"icon"`

	// Endpoints are included in the app response for easier discovery. The OAuth2
	// spec does not have a defined place to find these (for comparison, OIDC has
	// a '/.well-known/openid-configuration' endpoint).
	Endpoints OAuth2AppEndpoints `json:"endpoints"`
}

type OAuth2AppEndpoints struct {
	Authorization string `json:"authorization"`
	Token         string `json:"token"`
	// DeviceAuth is the device authorization endpoint for RFC 8628.
	DeviceAuth string `json:"device_authorization"`
	Revocation string `json:"revocation"`
}

type OAuth2ProviderAppFilter struct {
	UserID uuid.UUID `json:"user_id,omitempty" format:"uuid"`
}

// OAuth2ProviderApps returns the applications configured to authenticate using
// Coder as an OAuth2 provider.
func (c *Client) OAuth2ProviderApps(ctx context.Context, filter OAuth2ProviderAppFilter) ([]OAuth2ProviderApp, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/oauth2-provider/apps", nil,
		func(r *http.Request) {
			if filter.UserID != uuid.Nil {
				q := r.URL.Query()
				q.Set("user_id", filter.UserID.String())
				r.URL.RawQuery = q.Encode()
			}
		})
	if err != nil {
		return []OAuth2ProviderApp{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return []OAuth2ProviderApp{}, ReadBodyAsError(res)
	}
	var apps []OAuth2ProviderApp
	return apps, json.NewDecoder(res.Body).Decode(&apps)
}

// OAuth2ProviderApp returns an application configured to authenticate using
// Coder as an OAuth2 provider.
func (c *Client) OAuth2ProviderApp(ctx context.Context, id uuid.UUID) (OAuth2ProviderApp, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/oauth2-provider/apps/%s", id), nil)
	if err != nil {
		return OAuth2ProviderApp{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OAuth2ProviderApp{}, ReadBodyAsError(res)
	}
	var apps OAuth2ProviderApp
	return apps, json.NewDecoder(res.Body).Decode(&apps)
}

type PostOAuth2ProviderAppRequest struct {
	Name         string   `json:"name" validate:"required,oauth2_app_display_name"`
	RedirectURIs []string `json:"redirect_uris" validate:"required,min=1,dive,http_url"`
	Icon         string   `json:"icon" validate:"omitempty"`
}

// PostOAuth2ProviderApp adds an application that can authenticate using Coder
// as an OAuth2 provider.
func (c *Client) PostOAuth2ProviderApp(ctx context.Context, app PostOAuth2ProviderAppRequest) (OAuth2ProviderApp, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/oauth2-provider/apps", app)
	if err != nil {
		return OAuth2ProviderApp{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return OAuth2ProviderApp{}, ReadBodyAsError(res)
	}
	var resp OAuth2ProviderApp
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

type PutOAuth2ProviderAppRequest struct {
	Name         string   `json:"name" validate:"required,oauth2_app_display_name"`
	RedirectURIs []string `json:"redirect_uris" validate:"required,min=1,dive,http_url"`
	Icon         string   `json:"icon" validate:"omitempty"`
}

// PutOAuth2ProviderApp updates an application that can authenticate using Coder
// as an OAuth2 provider.
func (c *Client) PutOAuth2ProviderApp(ctx context.Context, id uuid.UUID, app PutOAuth2ProviderAppRequest) (OAuth2ProviderApp, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/oauth2-provider/apps/%s", id), app)
	if err != nil {
		return OAuth2ProviderApp{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OAuth2ProviderApp{}, ReadBodyAsError(res)
	}
	var resp OAuth2ProviderApp
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DeleteOAuth2ProviderApp deletes an application, also invalidating any tokens
// that were generated from it.
func (c *Client) DeleteOAuth2ProviderApp(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/oauth2-provider/apps/%s", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

type OAuth2ProviderAppSecretFull struct {
	ID               uuid.UUID `json:"id" format:"uuid"`
	ClientSecretFull string    `json:"client_secret_full"`
}

type OAuth2ProviderAppSecret struct {
	ID                    uuid.UUID `json:"id" format:"uuid"`
	LastUsedAt            NullTime  `json:"last_used_at"`
	ClientSecretTruncated string    `json:"client_secret_truncated"`
}

// OAuth2ProviderAppSecrets returns the truncated secrets for an OAuth2
// application.
func (c *Client) OAuth2ProviderAppSecrets(ctx context.Context, appID uuid.UUID) ([]OAuth2ProviderAppSecret, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/oauth2-provider/apps/%s/secrets", appID), nil)
	if err != nil {
		return []OAuth2ProviderAppSecret{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return []OAuth2ProviderAppSecret{}, ReadBodyAsError(res)
	}
	var resp []OAuth2ProviderAppSecret
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// PostOAuth2ProviderAppSecret creates a new secret for an OAuth2 application.
// This is the only time the full secret will be revealed.
func (c *Client) PostOAuth2ProviderAppSecret(ctx context.Context, appID uuid.UUID) (OAuth2ProviderAppSecretFull, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/oauth2-provider/apps/%s/secrets", appID), nil)
	if err != nil {
		return OAuth2ProviderAppSecretFull{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return OAuth2ProviderAppSecretFull{}, ReadBodyAsError(res)
	}
	var resp OAuth2ProviderAppSecretFull
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DeleteOAuth2ProviderAppSecret deletes a secret from an OAuth2 application,
// also invalidating any tokens that generated from it.
func (c *Client) DeleteOAuth2ProviderAppSecret(ctx context.Context, appID uuid.UUID, secretID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/oauth2-provider/apps/%s/secrets/%s", appID, secretID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

type OAuth2ProviderGrantType string

const (
	OAuth2ProviderGrantTypeAuthorizationCode OAuth2ProviderGrantType = "authorization_code"
	OAuth2ProviderGrantTypeRefreshToken      OAuth2ProviderGrantType = "refresh_token"
	OAuth2ProviderGrantTypeDeviceCode        OAuth2ProviderGrantType = "urn:ietf:params:oauth:grant-type:device_code"
)

func (e OAuth2ProviderGrantType) Valid() bool {
	switch e {
	case OAuth2ProviderGrantTypeAuthorizationCode, OAuth2ProviderGrantTypeRefreshToken, OAuth2ProviderGrantTypeDeviceCode:
		return true
	}
	return false
}

type OAuth2ProviderResponseType string

const (
	OAuth2ProviderResponseTypeCode OAuth2ProviderResponseType = "code"
)

func (e OAuth2ProviderResponseType) Valid() bool {
	//nolint:gocritic,revive // More cases might be added later.
	switch e {
	case OAuth2ProviderResponseTypeCode:
		return true
	}
	return false
}

// RevokeOAuth2Token revokes a specific OAuth2 token using RFC 7009 token revocation.
func (c *Client) RevokeOAuth2Token(ctx context.Context, clientID, token, tokenTypeHint string) error {
	form := url.Values{}
	form.Set("token", token)
	if tokenTypeHint != "" {
		form.Set("token_type_hint", tokenTypeHint)
	}
	// Client authentication is handled via the client_id in the app middleware
	form.Set("client_id", clientID)

	res, err := c.Request(ctx, http.MethodPost, "/oauth2/revoke", strings.NewReader(form.Encode()), func(r *http.Request) {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

type OAuth2DeviceFlowCallbackResponse struct {
	RedirectURL string `json:"redirect_url"`
}

// OAuth2AuthorizationServerMetadata represents RFC 8414 OAuth 2.0 Authorization Server Metadata
type OAuth2AuthorizationServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	DeviceAuthorizationEndpoint       string   `json:"device_authorization_endpoint,omitempty"` // RFC 8628
	RegistrationEndpoint              string   `json:"registration_endpoint,omitempty"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	ScopesSupported                   []string `json:"scopes_supported,omitempty"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported,omitempty"`
}

// OAuth2ProtectedResourceMetadata represents RFC 9728 OAuth 2.0 Protected Resource Metadata
type OAuth2ProtectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
}

// OAuth2ClientRegistrationRequest represents RFC 7591 Dynamic Client Registration Request
type OAuth2ClientRegistrationRequest struct {
	RedirectURIs            []string        `json:"redirect_uris,omitempty"`
	ClientName              string          `json:"client_name,omitempty"`
	ClientURI               string          `json:"client_uri,omitempty"`
	LogoURI                 string          `json:"logo_uri,omitempty"`
	TOSURI                  string          `json:"tos_uri,omitempty"`
	PolicyURI               string          `json:"policy_uri,omitempty"`
	JWKSURI                 string          `json:"jwks_uri,omitempty"`
	JWKS                    json.RawMessage `json:"jwks,omitempty" swaggertype:"object"`
	SoftwareID              string          `json:"software_id,omitempty"`
	SoftwareVersion         string          `json:"software_version,omitempty"`
	SoftwareStatement       string          `json:"software_statement,omitempty"`
	GrantTypes              []string        `json:"grant_types,omitempty"`
	ResponseTypes           []string        `json:"response_types,omitempty"`
	TokenEndpointAuthMethod string          `json:"token_endpoint_auth_method,omitempty"`
	Scope                   string          `json:"scope,omitempty"`
	Contacts                []string        `json:"contacts,omitempty"`
}

func (req OAuth2ClientRegistrationRequest) ApplyDefaults() OAuth2ClientRegistrationRequest {
	// Apply grant type defaults
	if len(req.GrantTypes) == 0 {
		req.GrantTypes = []string{
			string(OAuth2ProviderGrantTypeAuthorizationCode),
			string(OAuth2ProviderGrantTypeRefreshToken),
		}
	}

	// Apply response type defaults
	if len(req.ResponseTypes) == 0 {
		req.ResponseTypes = []string{
			string(OAuth2ProviderResponseTypeCode),
		}
	}

	// Apply token endpoint auth method default (RFC 7591 section 2)
	if req.TokenEndpointAuthMethod == "" {
		// Default according to RFC 7591: "client_secret_basic" for confidential clients
		// For public clients, should be explicitly set to "none"
		req.TokenEndpointAuthMethod = "client_secret_basic"
	}

	// Apply client name default if not provided
	if req.ClientName == "" {
		req.ClientName = "Dynamically Registered Client"
	}

	return req
}

// DetermineClientType determines if client is public or confidential
func (*OAuth2ClientRegistrationRequest) DetermineClientType() string {
	// For now, default to confidential
	// In the future, we might detect based on:
	// - token_endpoint_auth_method == "none" -> public
	// - application_type == "native" -> might be public
	// - Other heuristics
	return "confidential"
}

// GenerateClientName generates a client name if not provided
func (req *OAuth2ClientRegistrationRequest) GenerateClientName() string {
	if req.ClientName != "" {
		// Ensure client name fits database constraint (varchar(64))
		if len(req.ClientName) > 64 {
			// Preserve uniqueness by including a hash of the original name
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte(req.ClientName)))[:8]
			maxPrefix := 64 - 1 - len(hash) // 1 for separator
			return req.ClientName[:maxPrefix] + "-" + hash
		}
		return req.ClientName
	}

	// Try to derive from client_uri
	if req.ClientURI != "" {
		if uri, err := url.Parse(req.ClientURI); err == nil && uri.Host != "" {
			name := fmt.Sprintf("Client (%s)", uri.Host)
			if len(name) > 64 {
				return name[:64]
			}
			return name
		}
	}

	// Try to derive from first redirect URI
	if len(req.RedirectURIs) > 0 {
		if uri, err := url.Parse(req.RedirectURIs[0]); err == nil && uri.Host != "" {
			name := fmt.Sprintf("Client (%s)", uri.Host)
			if len(name) > 64 {
				return name[:64]
			}
			return name
		}
	}

	return "Dynamically Registered Client"
}

// OAuth2ClientRegistrationResponse represents RFC 7591 Dynamic Client Registration Response
type OAuth2ClientRegistrationResponse struct {
	ClientID                string          `json:"client_id"`
	ClientSecret            string          `json:"client_secret,omitempty"`
	ClientIDIssuedAt        int64           `json:"client_id_issued_at"`
	ClientSecretExpiresAt   int64           `json:"client_secret_expires_at,omitempty"`
	RedirectURIs            []string        `json:"redirect_uris,omitempty"`
	ClientName              string          `json:"client_name,omitempty"`
	ClientURI               string          `json:"client_uri,omitempty"`
	LogoURI                 string          `json:"logo_uri,omitempty"`
	TOSURI                  string          `json:"tos_uri,omitempty"`
	PolicyURI               string          `json:"policy_uri,omitempty"`
	JWKSURI                 string          `json:"jwks_uri,omitempty"`
	JWKS                    json.RawMessage `json:"jwks,omitempty" swaggertype:"object"`
	SoftwareID              string          `json:"software_id,omitempty"`
	SoftwareVersion         string          `json:"software_version,omitempty"`
	GrantTypes              []string        `json:"grant_types"`
	ResponseTypes           []string        `json:"response_types"`
	TokenEndpointAuthMethod string          `json:"token_endpoint_auth_method"`
	Scope                   string          `json:"scope,omitempty"`
	Contacts                []string        `json:"contacts,omitempty"`
	RegistrationAccessToken string          `json:"registration_access_token"`
	RegistrationClientURI   string          `json:"registration_client_uri"`
}

// PostOAuth2ClientRegistration dynamically registers a new OAuth2 client (RFC 7591)
func (c *Client) PostOAuth2ClientRegistration(ctx context.Context, req OAuth2ClientRegistrationRequest) (OAuth2ClientRegistrationResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/oauth2/register", req)
	if err != nil {
		return OAuth2ClientRegistrationResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return OAuth2ClientRegistrationResponse{}, ReadBodyAsError(res)
	}
	var resp OAuth2ClientRegistrationResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// GetOAuth2ClientConfiguration retrieves client configuration (RFC 7592)
func (c *Client) GetOAuth2ClientConfiguration(ctx context.Context, clientID string, registrationAccessToken string) (OAuth2ClientConfiguration, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/oauth2/clients/%s", clientID), nil,
		func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+registrationAccessToken)
		})
	if err != nil {
		return OAuth2ClientConfiguration{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OAuth2ClientConfiguration{}, ReadBodyAsError(res)
	}
	var resp OAuth2ClientConfiguration
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// PutOAuth2ClientConfiguration updates client configuration (RFC 7592)
func (c *Client) PutOAuth2ClientConfiguration(ctx context.Context, clientID string, registrationAccessToken string, req OAuth2ClientRegistrationRequest) (OAuth2ClientConfiguration, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/oauth2/clients/%s", clientID), req,
		func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+registrationAccessToken)
		})
	if err != nil {
		return OAuth2ClientConfiguration{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OAuth2ClientConfiguration{}, ReadBodyAsError(res)
	}
	var resp OAuth2ClientConfiguration
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DeleteOAuth2ClientConfiguration deletes client registration (RFC 7592)
func (c *Client) DeleteOAuth2ClientConfiguration(ctx context.Context, clientID string, registrationAccessToken string) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/oauth2/clients/%s", clientID), nil,
		func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+registrationAccessToken)
		})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// PostOAuth2DeviceAuthorization initiates RFC 8628 Device Authorization Grant flow
func (c *Client) PostOAuth2DeviceAuthorization(ctx context.Context, req OAuth2DeviceAuthorizationRequest) (OAuth2DeviceAuthorizationResponse, error) {
	form := url.Values{
		"client_id": {req.ClientID},
	}
	if req.Scope != "" {
		form.Set("scope", req.Scope)
	}
	if req.Resource != "" {
		form.Set("resource", req.Resource)
	}

	res, err := c.Request(ctx, http.MethodPost, "/oauth2/device", nil, func(r *http.Request) {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	})
	if err != nil {
		return OAuth2DeviceAuthorizationResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OAuth2DeviceAuthorizationResponse{}, ReadBodyAsError(res)
	}
	var resp OAuth2DeviceAuthorizationResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// PostOAuth2DeviceVerification processes device verification (authorize/deny)
func (c *Client) PostOAuth2DeviceVerification(ctx context.Context, req OAuth2DeviceVerificationRequest, action string) error {
	switch action {
	case oauth2DeviceActionAuthorize, oauth2DeviceActionDeny:
	default:
		return xerrors.Errorf("invalid action %q", action)
	}
	form := url.Values{
		"user_code": {req.UserCode},
		"action":    {action},
	}

	res, err := c.Request(ctx, http.MethodPost, "/oauth2/device/verify", nil, func(r *http.Request) {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

// PostOAuth2TokenExchange exchanges various grants for OAuth2 tokens
func (c *Client) PostOAuth2TokenExchange(ctx context.Context, form url.Values) (*oauth2.Token, error) {
	res, err := c.Request(ctx, http.MethodPost, "/oauth2/token", nil, func(r *http.Request) {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Body = io.NopCloser(strings.NewReader(form.Encode()))
	})
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var token oauth2.Token
	return &token, json.NewDecoder(res.Body).Decode(&token)
}

// GetOAuth2AuthorizationServerMetadata returns OAuth2 authorization server metadata
func (c *Client) GetOAuth2AuthorizationServerMetadata(ctx context.Context) (OAuth2AuthorizationServerMetadata, error) {
	res, err := c.Request(ctx, http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	if err != nil {
		return OAuth2AuthorizationServerMetadata{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OAuth2AuthorizationServerMetadata{}, ReadBodyAsError(res)
	}
	var metadata OAuth2AuthorizationServerMetadata
	return metadata, json.NewDecoder(res.Body).Decode(&metadata)
}

// OAuth2ClientConfiguration represents RFC 7592 Client Configuration (for GET/PUT operations)
// Same as OAuth2ClientRegistrationResponse but without client_secret in GET responses
type OAuth2ClientConfiguration struct {
	ClientID                string          `json:"client_id"`
	ClientIDIssuedAt        int64           `json:"client_id_issued_at"`
	ClientSecretExpiresAt   int64           `json:"client_secret_expires_at,omitempty"`
	RedirectURIs            []string        `json:"redirect_uris,omitempty"`
	ClientName              string          `json:"client_name,omitempty"`
	ClientURI               string          `json:"client_uri,omitempty"`
	LogoURI                 string          `json:"logo_uri,omitempty"`
	TOSURI                  string          `json:"tos_uri,omitempty"`
	PolicyURI               string          `json:"policy_uri,omitempty"`
	JWKSURI                 string          `json:"jwks_uri,omitempty"`
	JWKS                    json.RawMessage `json:"jwks,omitempty" swaggertype:"object"`
	SoftwareID              string          `json:"software_id,omitempty"`
	SoftwareVersion         string          `json:"software_version,omitempty"`
	GrantTypes              []string        `json:"grant_types"`
	ResponseTypes           []string        `json:"response_types"`
	TokenEndpointAuthMethod string          `json:"token_endpoint_auth_method"`
	Scope                   string          `json:"scope,omitempty"`
	Contacts                []string        `json:"contacts,omitempty"`
	RegistrationAccessToken string          `json:"registration_access_token"`
	RegistrationClientURI   string          `json:"registration_client_uri"`
}

// OAuth2DeviceAuthorizationRequest represents RFC 8628 Device Authorization Request
type OAuth2DeviceAuthorizationRequest struct {
	ClientID string `json:"client_id" validate:"required"`
	Scope    string `json:"scope,omitempty"`
	Resource string `json:"resource,omitempty"` // RFC 8707 resource parameter
}

// OAuth2DeviceAuthorizationResponse represents RFC 8628 Device Authorization Response
type OAuth2DeviceAuthorizationResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int64  `json:"interval,omitempty"`
}

// OAuth2DeviceVerificationRequest represents the user input for device verification
type OAuth2DeviceVerificationRequest struct {
	UserCode string `json:"user_code" validate:"required"`
}
