package oauth2dcr

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

var ErrDynamicClientRegistrationNotSupported = xerrors.New("dynamic client registration not supported by server")

// OAuth2ClientRegistrationRequest is a request to register a new client.
// See: https://datatracker.ietf.org/doc/html/rfc7591#section-2
//
// Intentionally excluded fields from RFC 7591:
// - jwks_uri
// - jwks
// - software_statement
type OAuth2ClientRegistrationRequest struct {
	// RedirectURIs is an array of redirection URI strings for use in
	// redirect-based flows such as the authorization code and implicit flows.
	// Clients using flows with redirection MUST register their redirection URI
	// values.
	RedirectURIs []string `json:"redirect_uris,omitempty"`
	// TokenEndpointAuthMethod is a string indicator of the requested
	// authentication method for the token endpoint.
	// Values defined by RFC 7591 are:
	// - none
	// - client_secret_post
	// - client_secret_basic (default)
	TokenEndpointAuthMethod string `json:"token_endpoint_auth_method,omitempty"`
	// GrantTypes is an array of OAuth 2.0 grant type strings that the client
	// can use at the token endpoint.
	// Values defined by RFC 7591 are:
	// - authorization_code (default)
	// - implicit
	// - password
	// - client_credentials
	// - refresh_token
	// - urn:ietf:params:oauth:grant-type:jwt-bearer
	// - urn:ietf:params:oauth:grant-type:saml2-bearer
	GrantTypes []string `json:"grant_types,omitempty"`
	// ResponseTypes is an array of the OAuth 2.0 response type strings that
	// the client can use at the authorization endpoint.
	// Values defined by RFC 7591 are:
	// - code (default)
	// - token
	ResponseTypes []string `json:"response_types,omitempty"`
	// ClientName is a human-readable string name of the client to be presented
	// to the end-user during authorization.
	ClientName string `json:"client_name,omitempty"`
	// ClientURI is a URL string of a web page providing information about the
	// client.
	ClientURI string `json:"client_uri,omitempty"`
	// LogoURI URL string that references a logo for the client. The value of
	// this field MUST point to a valid image file if set.
	LogoURI string `json:"logo_uri,omitempty"`
	// Scope is a space-separated list of scope values that the client can use
	// when requesting access tokens. The semantics of values in this list are
	// service specific.
	Scope string `json:"scope,omitempty"`
	// Contact is an array of strings representing ways to contact people
	// responsible for this client, typically email addresses.
	Contacts []string `json:"contacts,omitempty"`
	// TosURI is a URL string that points to a human-readable terms of service
	// document for the client that describes a contractual relationship between
	// the end-user and the client that the end-user accepts when authorizing
	// the client.
	TosURI string `json:"tos_uri,omitempty"`
	// PolicyURI is a URL string that points to a human-readable privacy policy
	// document that describes how the deployment organization collects, uses,
	// retains, and discloses personal data.
	PolicyURI string `json:"policy_uri,omitempty"`
	// SoftwareID is a unique identifier string (e.g., a Universally Unique
	// Identifier (UUID)) assigned by the client developer or software
	// publisher used by registration endpoints to identify the client
	// software to be dynamically registered.
	SoftwareID string `json:"software_id,omitempty"`
	// SoftwareVersion is a version identifier string for the client software
	// identified by "software_id". The value of the "software_version" SHOULD
	// change on any update to the client software identified by the same
	// "software_id".
	SoftwareVersion string `json:"software_version,omitempty"`
}

// OAuth2ClientRegistrationResponse is a response to a client registration
// request.
// See: https://datatracker.ietf.org/doc/html/rfc7591#section-3.2
type OAuth2ClientRegistrationResponse struct {
	// The server must return all registered metadata about the client,
	// including any fields provisioned by the authorization server itself. The
	// authorization server MAY reject or replace any of the client's requested
	// metadata values submitted during the registration and substitute them
	// with suitable values.
	OAuth2ClientRegistrationRequest

	// ClientID is the OAuth 2.0 client identifier string.
	ClientID string `json:"client_id"`
	// ClientSecret is the OAuth 2.0 client secret string.
	ClientSecret string `json:"client_secret,omitempty"`
	// ClientIDIssuedAt is the time at which the client identifier was issued.
	// Optional. Unix timestamp in seconds.
	ClientIDIssuedAt int64 `json:"client_id_issued_at,omitempty"`
	// ClientSecretExpiresAt is the time at which the client secret will expire
	// or 0 if it will not expire. Required if the client secret is included.
	// Unix timestamp in seconds.
	ClientSecretExpiresAt int64 `json:"client_secret_expires_at,omitempty"`
}

// RegisterDynamicClient attempts to register a dynamic client from the issuer
// with the given registration request. If the issuer does not support dynamic
// client registration, ErrDyanmicClientRegistrationNotSupported is returned.
func RegisterDynamicClientWithMetadata(ctx context.Context, client *http.Client, metadataResp OAuth2AuthorizationServerMetadataResponse, registrationRequest OAuth2ClientRegistrationRequest) (OAuth2ClientRegistrationResponse, error) {
	if metadataResp.RegistrationEndpoint == "" {
		return OAuth2ClientRegistrationResponse{}, ErrDynamicClientRegistrationNotSupported
	}

	// Verify the registration request is supported by the server.
	if len(metadataResp.ResponseTypesSupported) > 0 {
		for _, responseType := range registrationRequest.ResponseTypes {
			if !slices.Contains(metadataResp.ResponseTypesSupported, responseType) {
				return OAuth2ClientRegistrationResponse{}, xerrors.Errorf("metadata claims that server does not support response type %q", responseType)
			}
		}
	}
	if len(metadataResp.ScopesSupported) > 0 && registrationRequest.Scope != "" {
		for _, scope := range strings.Split(registrationRequest.Scope, " ") {
			if !slices.Contains(metadataResp.ScopesSupported, scope) {
				return OAuth2ClientRegistrationResponse{}, xerrors.Errorf("metadata claims that server does not support scope %q", scope)
			}
		}
	}
	if len(metadataResp.GrantTypesSupported) > 0 {
		for _, grantType := range registrationRequest.GrantTypes {
			if !slices.Contains(metadataResp.GrantTypesSupported, grantType) {
				return OAuth2ClientRegistrationResponse{}, xerrors.Errorf("metadata claims that server does not support grant type %q", grantType)
			}
		}
	}
	if len(metadataResp.TokenEndpointAuthMethodsSupported) > 0 && registrationRequest.TokenEndpointAuthMethod != "" && !slices.Contains(metadataResp.TokenEndpointAuthMethodsSupported, registrationRequest.TokenEndpointAuthMethod) {
		return OAuth2ClientRegistrationResponse{}, xerrors.Errorf("metadata claims that server does not support token endpoint auth method %q", registrationRequest.TokenEndpointAuthMethod)
	}

	registrationURL, err := url.Parse(metadataResp.RegistrationEndpoint)
	if err != nil {
		return OAuth2ClientRegistrationResponse{}, xerrors.Errorf("metadata contained invalid registration endpoint %q: %w", metadataResp.RegistrationEndpoint, err)
	}
	if registrationURL.Scheme != "https" {
		return OAuth2ClientRegistrationResponse{}, xerrors.Errorf("metadata contained registration endpoint URL that is not HTTPS: %q", metadataResp.RegistrationEndpoint)
	}

	var registrationResp OAuth2ClientRegistrationResponse
	registerReqCtx, registerReqCancel := context.WithTimeout(ctx, 15*time.Second)
	// nolint:bodyclose // makeJSONRequest closes the body
	resp, err := makeJSONRequest(registerReqCtx, client, http.MethodPost, registrationURL.String(), registrationRequest, &registrationResp)
	registerReqCancel()
	// Check the status code before checking error, since the error will mostly
	// likely be about it.
	if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
		// We consider this as the server not supporting dynamic client
		// registration.
		return OAuth2ClientRegistrationResponse{}, ErrDynamicClientRegistrationNotSupported
	}
	if err != nil {
		return OAuth2ClientRegistrationResponse{}, xerrors.Errorf("register client: %w", err)
	}

	if registrationResp.ClientID == "" {
		return OAuth2ClientRegistrationResponse{}, xerrors.New("registration endpoint did not return a client ID")
	}

	return registrationResp, nil
}

// RegisterDynamicClientWithIssuer fetches the OAuth 2.0 authorization server
// metadata from the given issuer URL and then registers a dynamic client with
// the given registration request. If the issuer metadata endpoint is not found,
// ErrMetadataEndpointNotFound is returned. If the issuer does not support
// dynamic client registration, ErrDyanmicClientRegistrationNotSupported is
// returned.
func RegisterDynamicClientWithIssuer(ctx context.Context, client *http.Client, issuerURL string, registrationRequest OAuth2ClientRegistrationRequest) (OAuth2ClientRegistrationResponse, error) {
	metadataResp, err := GetOAuth2AuthorizationServerMetadata(ctx, client, issuerURL)
	if err != nil {
		return OAuth2ClientRegistrationResponse{}, xerrors.Errorf("get oauth2 server metadata: %w", err)
	}

	return RegisterDynamicClientWithMetadata(ctx, client, metadataResp, registrationRequest)
}
