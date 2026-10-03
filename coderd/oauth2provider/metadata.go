package oauth2provider

import (
	"net/http"
	"net/url"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// GetAuthorizationServerMetadata returns an http.HandlerFunc that handles GET /.well-known/oauth-authorization-server
func GetAuthorizationServerMetadata(db database.Store, accessURL *url.URL) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// This is queried on every request rather than cached, for the
		// same reason as the registration endpoint: discovery is not
		// expected to be a hot path, and a flood of requests should be
		// mitigated with rate limiting or firewalling, not a cache.
		//nolint:gocritic // Public discovery endpoint, no authenticated actor to authorize against.
		dcrEnabled, err := db.GetOAuth2DCREnabled(dbauthz.AsSystemOAuth2(ctx))
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}

		metadata := codersdk.OAuth2AuthorizationServerMetadata{
			Issuer:                        accessURL.String(),
			AuthorizationEndpoint:         accessURL.JoinPath("/oauth2/authorize").String(),
			TokenEndpoint:                 accessURL.JoinPath("/oauth2/tokens").String(),
			RevocationEndpoint:            accessURL.JoinPath("/oauth2/revoke").String(), // RFC 7009
			ResponseTypesSupported:        []codersdk.OAuth2ProviderResponseType{codersdk.OAuth2ProviderResponseTypeCode},
			GrantTypesSupported:           []codersdk.OAuth2ProviderGrantType{codersdk.OAuth2ProviderGrantTypeAuthorizationCode, codersdk.OAuth2ProviderGrantTypeRefreshToken},
			CodeChallengeMethodsSupported: []codersdk.OAuth2PKCECodeChallengeMethod{codersdk.OAuth2PKCECodeChallengeMethodS256},
			ScopesSupported:               rbac.ExternalScopeNames(),
			// Not gated on dcrEnabled: existing clients still need to
			// exchange tokens when new registrations are turned off.
			TokenEndpointAuthMethodsSupported: codersdk.AdvertisedOAuth2TokenEndpointAuthMethods(),
		}
		if dcrEnabled {
			metadata.RegistrationEndpoint = accessURL.JoinPath("/oauth2/register").String() // RFC 7591
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
