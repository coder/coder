package oauth2provider

import (
	"net/http"
	"net/url"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// GetAuthorizationServerMetadata returns an http.HandlerFunc that handles GET /.well-known/oauth-authorization-server
func GetAuthorizationServerMetadata(accessURL *url.URL) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		metadata := codersdk.OAuth2AuthorizationServerMetadata{
			Issuer:                        accessURL.String(),
			AuthorizationEndpoint:         accessURL.JoinPath("/oauth2/authorize").String(),
			TokenEndpoint:                 accessURL.JoinPath("/oauth2/tokens").String(),
			RegistrationEndpoint:          accessURL.JoinPath("/oauth2/register").String(), // RFC 7591
			ResponseTypesSupported:        []string{"code"},
			GrantTypesSupported:           []string{"authorization_code", "refresh_token"},
			CodeChallengeMethodsSupported: []string{"S256"},
			// TODO: Implement scope system
			ScopesSupported:                   []string{},
			TokenEndpointAuthMethodsSupported: []string{"client_secret_post"},
		}
		httpapi.Write(ctx, rw, http.StatusOK, metadata)
	}
}

// GetProtectedResourceMetadata returns an http.HandlerFunc that handles GET /.well-known/oauth-protected-resource
func GetProtectedResourceMetadata(accessURL *url.URL) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		metadata := codersdk.OAuth2ProtectedResourceMetadata{
			Resource:             accessURL.String(),
			AuthorizationServers: []string{accessURL.String()},
			// TODO: Implement scope system based on RBAC permissions
			ScopesSupported: []string{},
			// RFC 6750 Bearer Token methods supported as fallback methods in api key middleware
			BearerMethodsSupported: []string{"header", "query"},
		}
		httpapi.Write(ctx, rw, http.StatusOK, metadata)
	}
}
