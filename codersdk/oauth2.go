package codersdk

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

type OAuth2ProviderApp struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	CallbackURL string    `json:"callback_url"`
	Icon        string    `json:"icon"`

	// Endpoints are included in the app response for easier discovery. The OAuth2
	// spec does not have a defined place to find these (for comparison, OIDC has
	// a '/.well-known/openid-configuration' endpoint).
	Endpoints OAuth2AppEndpoints `json:"endpoints"`
}

type OAuth2AppEndpoints struct {
	Authorization string `json:"authorization"`
	Token         string `json:"token"`
	TokenRevoke   string `json:"token_revoke"`
	// DeviceAuth is optional.
	DeviceAuth string `json:"device_authorization"`
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
	Name        string `json:"name" validate:"required,oauth2_app_name"`
	CallbackURL string `json:"callback_url" validate:"required,http_url"`
	Icon        string `json:"icon" validate:"omitempty"`
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
	Name        string `json:"name" validate:"required,oauth2_app_name"`
	CallbackURL string `json:"callback_url" validate:"required,http_url"`
	Icon        string `json:"icon" validate:"omitempty"`
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
)

func (e OAuth2ProviderGrantType) Valid() bool {
	switch e {
	case OAuth2ProviderGrantTypeAuthorizationCode, OAuth2ProviderGrantTypeRefreshToken:
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

type OAuth2TokenEndpointAuthMethod string

const (
	OAuth2TokenEndpointAuthMethodClientSecretBasic OAuth2TokenEndpointAuthMethod = "client_secret_basic"
	OAuth2TokenEndpointAuthMethodClientSecretPost  OAuth2TokenEndpointAuthMethod = "client_secret_post"
	OAuth2TokenEndpointAuthMethodNone              OAuth2TokenEndpointAuthMethod = "none"
)

func (m OAuth2TokenEndpointAuthMethod) Valid() bool {
	switch m {
	case OAuth2TokenEndpointAuthMethodClientSecretBasic,
		OAuth2TokenEndpointAuthMethodClientSecretPost,
		OAuth2TokenEndpointAuthMethodNone:
		return true
	}
	return false
}

// OAuth 2.1 only supports S256 (plain is rejected).
type OAuth2PKCECodeChallengeMethod string

const (
	OAuth2PKCECodeChallengeMethodS256 OAuth2PKCECodeChallengeMethod = "S256"
)

func (m OAuth2PKCECodeChallengeMethod) Valid() bool {
	return m == OAuth2PKCECodeChallengeMethodS256
}

type OAuth2TokenType string

const (
	OAuth2TokenTypeBearer OAuth2TokenType = "Bearer"
)

func (t OAuth2TokenType) Valid() bool {
	//nolint:gocritic,revive // More cases might be added later.
	switch t {
	case OAuth2TokenTypeBearer:
		return true
	}
	return false
}

type OAuth2RevocationTokenTypeHint string

const (
	OAuth2RevocationTokenTypeHintAccessToken  OAuth2RevocationTokenTypeHint = "access_token"
	OAuth2RevocationTokenTypeHintRefreshToken OAuth2RevocationTokenTypeHint = "refresh_token"
)

func (h OAuth2RevocationTokenTypeHint) Valid() bool {
	switch h {
	case OAuth2RevocationTokenTypeHintAccessToken, OAuth2RevocationTokenTypeHintRefreshToken:
		return true
	}
	return false
}

type OAuth2ErrorCode string

const (
	OAuth2ErrorCodeInvalidRequest         OAuth2ErrorCode = "invalid_request"
	OAuth2ErrorCodeInvalidClient          OAuth2ErrorCode = "invalid_client"
	OAuth2ErrorCodeInvalidGrant           OAuth2ErrorCode = "invalid_grant"
	OAuth2ErrorCodeUnauthorizedClient     OAuth2ErrorCode = "unauthorized_client"
	OAuth2ErrorCodeUnsupportedGrantType   OAuth2ErrorCode = "unsupported_grant_type"
	OAuth2ErrorCodeInvalidScope           OAuth2ErrorCode = "invalid_scope"
	OAuth2ErrorCodeInvalidTarget          OAuth2ErrorCode = "invalid_target"         // RFC 8707
	OAuth2ErrorCodeUnsupportedTokenType   OAuth2ErrorCode = "unsupported_token_type" // RFC 7009
	OAuth2ErrorCodeServerError            OAuth2ErrorCode = "server_error"
	OAuth2ErrorCodeTemporarilyUnavailable OAuth2ErrorCode = "temporarily_unavailable"
)

func (c OAuth2ErrorCode) Valid() bool {
	switch c {
	case OAuth2ErrorCodeInvalidRequest,
		OAuth2ErrorCodeInvalidClient,
		OAuth2ErrorCodeInvalidGrant,
		OAuth2ErrorCodeUnauthorizedClient,
		OAuth2ErrorCodeUnsupportedGrantType,
		OAuth2ErrorCodeInvalidScope,
		OAuth2ErrorCodeInvalidTarget,
		OAuth2ErrorCodeUnsupportedTokenType,
		OAuth2ErrorCodeServerError,
		OAuth2ErrorCodeTemporarilyUnavailable:
		return true
	}
	return false
}

// OAuth2Error represents an OAuth2-compliant error response per RFC 6749.
type OAuth2Error struct {
	Error            OAuth2ErrorCode `json:"error"`
	ErrorDescription string          `json:"error_description,omitempty"`
	ErrorURI         string          `json:"error_uri,omitempty"`
}

// OAuth2TokenRequest represents a token request per RFC 6749. The actual wire
// format is application/x-www-form-urlencoded; this struct is for SDK docs.
type OAuth2TokenRequest struct {
	GrantType    OAuth2ProviderGrantType `json:"grant_type"`
	Code         string                  `json:"code,omitempty"`
	RedirectURI  string                  `json:"redirect_uri,omitempty"`
	ClientID     string                  `json:"client_id,omitempty"`
	ClientSecret string                  `json:"client_secret,omitempty"`
	CodeVerifier string                  `json:"code_verifier,omitempty"`
	RefreshToken string                  `json:"refresh_token,omitempty"`
	Resource     string                  `json:"resource,omitempty"`
	Scope        string                  `json:"scope,omitempty"`
}

// OAuth2TokenResponse represents a successful token response per RFC 6749.
type OAuth2TokenResponse struct {
	AccessToken  string          `json:"access_token"`
	TokenType    OAuth2TokenType `json:"token_type"`
	ExpiresIn    int64           `json:"expires_in,omitempty"`
	RefreshToken string          `json:"refresh_token,omitempty"`
	Scope        string          `json:"scope,omitempty"`
	// Expiry is not part of RFC 6749 but is included for compatibility with
	// golang.org/x/oauth2.Token and clients that expect a timestamp.
	Expiry *time.Time `json:"expiry,omitempty" format:"date-time"`
}

// OAuth2TokenRevocationRequest represents a token revocation request per RFC 7009.
type OAuth2TokenRevocationRequest struct {
	Token         string                        `json:"token"`
	TokenTypeHint OAuth2RevocationTokenTypeHint `json:"token_type_hint,omitempty"`
	ClientID      string                        `json:"client_id,omitempty"`
	ClientSecret  string                        `json:"client_secret,omitempty"`
}

// RevokeOAuth2Token revokes a specific OAuth2 token using RFC 7009 token revocation.
func (c *Client) RevokeOAuth2Token(ctx context.Context, clientID uuid.UUID, token string) error {
	form := url.Values{}
	form.Set("token", token)
	// Client authentication is handled via the client_id in the app middleware
	form.Set("client_id", clientID.String())

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

// RevokeOAuth2ProviderApp completely revokes an app's access for the
// authenticated user.
func (c *Client) RevokeOAuth2ProviderApp(ctx context.Context, appID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, "/oauth2/tokens", nil, func(r *http.Request) {
		q := r.URL.Query()
		q.Set("client_id", appID.String())
		r.URL.RawQuery = q.Encode()
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

type OAuth2DeviceFlowCallbackResponse struct {
	RedirectURL string `json:"redirect_url"`
}

// OAuth2AuthorizationServerMetadata represents RFC 8414 OAuth 2.0 Authorization Server Metadata.
type OAuth2AuthorizationServerMetadata struct {
	Issuer                            string                          `json:"issuer"`
	AuthorizationEndpoint             string                          `json:"authorization_endpoint"`
	TokenEndpoint                     string                          `json:"token_endpoint"`
	RegistrationEndpoint              string                          `json:"registration_endpoint,omitempty"`
	RevocationEndpoint                string                          `json:"revocation_endpoint,omitempty"`
	ResponseTypesSupported            []OAuth2ProviderResponseType    `json:"response_types_supported"`
	GrantTypesSupported               []OAuth2ProviderGrantType       `json:"grant_types_supported,omitempty"`
	CodeChallengeMethodsSupported     []OAuth2PKCECodeChallengeMethod `json:"code_challenge_methods_supported,omitempty"`
	ScopesSupported                   []string                        `json:"scopes_supported,omitempty"`
	TokenEndpointAuthMethodsSupported []OAuth2TokenEndpointAuthMethod `json:"token_endpoint_auth_methods_supported,omitempty"`
}

// OAuth2ProtectedResourceMetadata represents RFC 9728 OAuth 2.0 Protected Resource Metadata
type OAuth2ProtectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
}

// OAuth2ClientRegistrationRequest represents RFC 7591 Dynamic Client Registration Request.
type OAuth2ClientRegistrationRequest struct {
	RedirectURIs            []string                      `json:"redirect_uris,omitempty"`
	ClientName              string                        `json:"client_name,omitempty"`
	ClientURI               string                        `json:"client_uri,omitempty"`
	LogoURI                 string                        `json:"logo_uri,omitempty"`
	TOSURI                  string                        `json:"tos_uri,omitempty"`
	PolicyURI               string                        `json:"policy_uri,omitempty"`
	JWKSURI                 string                        `json:"jwks_uri,omitempty"`
	JWKS                    json.RawMessage               `json:"jwks,omitempty" swaggertype:"object"`
	SoftwareID              string                        `json:"software_id,omitempty"`
	SoftwareVersion         string                        `json:"software_version,omitempty"`
	SoftwareStatement       string                        `json:"software_statement,omitempty"`
	GrantTypes              []OAuth2ProviderGrantType     `json:"grant_types,omitempty"`
	ResponseTypes           []OAuth2ProviderResponseType  `json:"response_types,omitempty"`
	TokenEndpointAuthMethod OAuth2TokenEndpointAuthMethod `json:"token_endpoint_auth_method,omitempty"`
	Scope                   string                        `json:"scope,omitempty"`
	Contacts                []string                      `json:"contacts,omitempty"`
}

func (req OAuth2ClientRegistrationRequest) ApplyDefaults() OAuth2ClientRegistrationRequest {
	// Apply grant type defaults.
	if len(req.GrantTypes) == 0 {
		req.GrantTypes = []OAuth2ProviderGrantType{
			OAuth2ProviderGrantTypeAuthorizationCode,
			OAuth2ProviderGrantTypeRefreshToken,
		}
	}

	// Apply response type defaults.
	if len(req.ResponseTypes) == 0 {
		req.ResponseTypes = []OAuth2ProviderResponseType{
			OAuth2ProviderResponseTypeCode,
		}
	}

	// Apply token endpoint auth method default (RFC 7591 section 2).
	if req.TokenEndpointAuthMethod == "" {
		// Default according to RFC 7591: "client_secret_basic" for confidential clients.
		// For public clients, should be explicitly set to "none".
		req.TokenEndpointAuthMethod = OAuth2TokenEndpointAuthMethodClientSecretBasic
	}

	// Apply client name default if not provided.
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

// OAuth2ClientRegistrationResponse represents RFC 7591 Dynamic Client Registration Response.
type OAuth2ClientRegistrationResponse struct {
	ClientID                string                        `json:"client_id"`
	ClientSecret            string                        `json:"client_secret,omitempty"`
	ClientIDIssuedAt        int64                         `json:"client_id_issued_at,omitempty"`
	ClientSecretExpiresAt   int64                         `json:"client_secret_expires_at,omitempty"`
	RedirectURIs            []string                      `json:"redirect_uris,omitempty"`
	ClientName              string                        `json:"client_name,omitempty"`
	ClientURI               string                        `json:"client_uri,omitempty"`
	LogoURI                 string                        `json:"logo_uri,omitempty"`
	TOSURI                  string                        `json:"tos_uri,omitempty"`
	PolicyURI               string                        `json:"policy_uri,omitempty"`
	JWKSURI                 string                        `json:"jwks_uri,omitempty"`
	JWKS                    json.RawMessage               `json:"jwks,omitempty" swaggertype:"object"`
	SoftwareID              string                        `json:"software_id,omitempty"`
	SoftwareVersion         string                        `json:"software_version,omitempty"`
	GrantTypes              []OAuth2ProviderGrantType     `json:"grant_types"`
	ResponseTypes           []OAuth2ProviderResponseType  `json:"response_types"`
	TokenEndpointAuthMethod OAuth2TokenEndpointAuthMethod `json:"token_endpoint_auth_method"`
	Scope                   string                        `json:"scope,omitempty"`
	Contacts                []string                      `json:"contacts,omitempty"`
	RegistrationAccessToken string                        `json:"registration_access_token"`
	RegistrationClientURI   string                        `json:"registration_client_uri"`
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

// OAuth2ClientConfiguration represents RFC 7592 Client Read Response.
type OAuth2ClientConfiguration struct {
	ClientID                string                        `json:"client_id"`
	ClientIDIssuedAt        int64                         `json:"client_id_issued_at"`
	ClientSecretExpiresAt   int64                         `json:"client_secret_expires_at,omitempty"`
	RedirectURIs            []string                      `json:"redirect_uris,omitempty"`
	ClientName              string                        `json:"client_name,omitempty"`
	ClientURI               string                        `json:"client_uri,omitempty"`
	LogoURI                 string                        `json:"logo_uri,omitempty"`
	TOSURI                  string                        `json:"tos_uri,omitempty"`
	PolicyURI               string                        `json:"policy_uri,omitempty"`
	JWKSURI                 string                        `json:"jwks_uri,omitempty"`
	JWKS                    json.RawMessage               `json:"jwks,omitempty" swaggertype:"object"`
	SoftwareID              string                        `json:"software_id,omitempty"`
	SoftwareVersion         string                        `json:"software_version,omitempty"`
	GrantTypes              []OAuth2ProviderGrantType     `json:"grant_types"`
	ResponseTypes           []OAuth2ProviderResponseType  `json:"response_types"`
	TokenEndpointAuthMethod OAuth2TokenEndpointAuthMethod `json:"token_endpoint_auth_method"`
	Scope                   string                        `json:"scope,omitempty"`
	Contacts                []string                      `json:"contacts,omitempty"`
	RegistrationAccessToken string                        `json:"registration_access_token,omitempty"`
	RegistrationClientURI   string                        `json:"registration_client_uri"`
}
