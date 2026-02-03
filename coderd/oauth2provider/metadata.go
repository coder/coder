package oauth2provider

import (
	"net/http"
	"net/url"

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
			TokenEndpointAuthMethodsSupported: []codersdk.OAuth2TokenEndpointAuthMethod{codersdk.OAuth2TokenEndpointAuthMethodClientSecretBasic, codersdk.OAuth2TokenEndpointAuthMethodClientSecretPost},
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
			ScopesSupported:      rbac.ExternalScopeNames(),
			// RFC 6750 Bearer Token methods supported as fallback methods in api key middleware
			BearerMethodsSupported: []string{"header", "query"},
		}
		httpapi.Write(ctx, rw, http.StatusOK, metadata)
	}
}
