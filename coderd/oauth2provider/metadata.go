package oauth2provider

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// GetAuthorizationServerMetadata returns an http.HandlerFunc that handles GET /.well-known/oauth-authorization-server
func GetAuthorizationServerMetadata(accessURL *url.URL) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		metadata := codersdk.OAuth2AuthorizationServerMetadata{
			Issuer:                            accessURL.String(),
			AuthorizationEndpoint:             accessURL.JoinPath("/oauth2/authorize").String(),
			TokenEndpoint:                     accessURL.JoinPath("/oauth2/tokens").String(),
			RegistrationEndpoint:              accessURL.JoinPath("/oauth2/register").String(), // RFC 7591
			RevocationEndpoint:                accessURL.JoinPath("/oauth2/revoke").String(),   // RFC 7009
			ResponseTypesSupported:            []codersdk.OAuth2ProviderResponseType{codersdk.OAuth2ProviderResponseTypeCode},
			GrantTypesSupported:               []codersdk.OAuth2ProviderGrantType{codersdk.OAuth2ProviderGrantTypeAuthorizationCode, codersdk.OAuth2ProviderGrantTypeRefreshToken},
			CodeChallengeMethodsSupported:     []codersdk.OAuth2PKCECodeChallengeMethod{codersdk.OAuth2PKCECodeChallengeMethodS256},
			ScopesSupported:                   rbac.ExternalScopeNames(),
			TokenEndpointAuthMethodsSupported: []codersdk.OAuth2TokenEndpointAuthMethod{codersdk.OAuth2TokenEndpointAuthMethodClientSecretPost},
		}
		httpapi.Write(ctx, rw, http.StatusOK, metadata)
	}
}

// GetProtectedResourceMetadata returns an http.HandlerFunc that handles GET /.well-known/oauth-protected-resource
// Per RFC 9728, the resource identifier MUST match the protected resource the client is trying to access.
// The endpoint supports path-based resource identification: requests to
// /.well-known/oauth-protected-resource/api/v2/users will return metadata with
// resource set to {accessURL}/api/v2/users.
func GetProtectedResourceMetadata(accessURL *url.URL) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Build resource URL from the request path.
		// The path comes in as /.well-known/oauth-protected-resource{/resource-path}
		// Strip the well-known prefix to get the actual resource path.
		resourceURL := *accessURL
		const wellKnownPrefix = "/.well-known/oauth-protected-resource"
		resourcePath := strings.TrimPrefix(r.URL.Path, wellKnownPrefix)
		if resourcePath != "" && resourcePath != "/" {
			resourceURL.Path = resourcePath
		}

		metadata := codersdk.OAuth2ProtectedResourceMetadata{
			Resource:             resourceURL.String(),
			AuthorizationServers: []string{accessURL.String()},
			ScopesSupported:      rbac.ExternalScopeNames(),
			// RFC 6750 Bearer Token methods supported as fallback methods in api key middleware
			BearerMethodsSupported: []string{"header", "query"},
		}
		httpapi.Write(ctx, rw, http.StatusOK, metadata)
	}
}
