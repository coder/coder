package oauth2dcr

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"golang.org/x/xerrors"
)

const wellKnownOauthAuthorizationServer = "/.well-known/oauth-authorization-server"

var ErrMetadataEndpointNotFound = xerrors.New("metadata endpoint not found, issuer may not be valid")

// OAuth2AuthorizationServerMetadataResponse is the response from the metadata
// endpoint.
// See: https://datatracker.ietf.org/doc/html/rfc8414#section-3
type OAuth2AuthorizationServerMetadataResponse struct {
	// Required:
	Issuer                 string   `json:"issuer"`
	ResponseTypesSupported []string `json:"response_types_supported"`

	// Optional:
	AuthorizationEndpoint                         string   `json:"authorization_endpoint,omitempty"`
	TokenEndpoint                                 string   `json:"token_endpoint,omitempty"`
	JwksURI                                       string   `json:"jwks_uri,omitempty"`
	RegistrationEndpoint                          string   `json:"registration_endpoint,omitempty"`
	ScopesSupported                               []string `json:"scopes_supported,omitempty"`
	ResponseModesSupported                        []string `json:"response_modes_supported,omitempty"`
	GrantTypesSupported                           []string `json:"grant_types_supported,omitempty"`
	TokenEndpointAuthMethodsSupported             []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	TokenEndpointAuthSigningAlgsSupported         []string `json:"token_endpoint_auth_signing_alg_values_supported,omitempty"`
	ServiceDocumentation                          string   `json:"service_documentation,omitempty"`
	OpPolicyURI                                   string   `json:"op_policy_uri,omitempty"`
	OpTosURI                                      string   `json:"op_tos_uri,omitempty"`
	RevocationEndpoint                            string   `json:"revocation_endpoint,omitempty"`
	RevocationEndpointAuthMethodsSupported        []string `json:"revocation_endpoint_auth_methods_supported,omitempty"`
	RevocationEndpointAuthSigningAlgsSupported    []string `json:"revocation_endpoint_auth_signing_alg_values_supported,omitempty"`
	IntrospectionEndpoint                         string   `json:"introspection_endpoint,omitempty"`
	IntrospectionEndpointAuthMethodsSupported     []string `json:"introspection_endpoint_auth_methods_supported,omitempty"`
	IntrospectionEndpointAuthSigningAlgsSupported []string `json:"introspection_endpoint_auth_signing_alg_values_supported,omitempty"`
	CodeChallengeMethodsSupported                 []string `json:"code_challenge_methods_supported,omitempty"`
}

// GetOAuth2AuthorizationServerMetadata fetches the OAuth 2.0 authorization
// server metadata from the given issuer URL. If the issuer URL could not be
// found, ErrMetadataEndpointNotFound is returned.
func GetOAuth2AuthorizationServerMetadata(ctx context.Context, client *http.Client, issuerURL string) (OAuth2AuthorizationServerMetadataResponse, error) {
	u, err := url.Parse(issuerURL)
	if err != nil {
		return OAuth2AuthorizationServerMetadataResponse{}, xerrors.Errorf("invalid server URL %q: %w", issuerURL, err)
	}
	if u.Scheme != "https" {
		return OAuth2AuthorizationServerMetadataResponse{}, xerrors.Errorf("server URL must be HTTPS: %q", issuerURL)
	}

	// RFC 8414 says the issuer identifier should be appended to the well-known
	// URL.
	metadataPath := wellKnownOauthAuthorizationServer
	if u.Path != "" {
		metadataPath = path.Join(wellKnownOauthAuthorizationServer, u.Path)
	}

	metadataURL, err := u.Parse(metadataPath)
	if err != nil {
		return OAuth2AuthorizationServerMetadataResponse{}, xerrors.Errorf("could not parse well-known metadata URL %q + %q: %w", issuerURL, metadataPath, err)
	}

	// Enforce a 10 second timeout.
	var metadataResp OAuth2AuthorizationServerMetadataResponse
	metadataReqCtx, metadataReqCancel := context.WithTimeout(ctx, 10*time.Second)
	// nolint:bodyclose // makeJSONRequest closes the body
	resp, err := makeJSONRequest(metadataReqCtx, client, http.MethodGet, metadataURL.String(), nil, &metadataResp)
	metadataReqCancel()

	// Check the status code before checking error, since the error will mostly
	// likely be about it.
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		// We consider this as the server not being a valid issuer.
		return OAuth2AuthorizationServerMetadataResponse{}, ErrMetadataEndpointNotFound
	}
	if err != nil {
		return OAuth2AuthorizationServerMetadataResponse{}, xerrors.Errorf("get oauth2 server metadata: %w", err)
	}

	if metadataResp.Issuer != issuerURL {
		return OAuth2AuthorizationServerMetadataResponse{}, xerrors.Errorf("metadata endpoint returned issuer URL that does not match provided issuer URL: got %q, expected %q", metadataResp.Issuer, issuerURL)
	}

	return metadataResp, nil
}

// makeJSONRequest makes a JSON request to the given URL and decodes the JSON
// response into the given out value. If the response is not a 2xx, an error is
// returned.
// The request body is closed automatically.
func makeJSONRequest(ctx context.Context, client *http.Client, method, requestURL string, body any, out any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, xerrors.Errorf("could not marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL, bodyReader)
	if err != nil {
		return nil, xerrors.Errorf("%s %q: could not create request: %w", method, requestURL, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return resp, xerrors.Errorf("%s %q: %w", method, requestURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, xerrors.Errorf("%s %q: unexpected status code: got %v, expected 2xx", method, requestURL, resp.StatusCode)
	}

	err = json.NewDecoder(resp.Body).Decode(out)
	if err != nil {
		return resp, xerrors.Errorf("%s %q: could not decode JSON response: %w", method, requestURL, err)
	}
	return resp, nil
}
