package coderd

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/oauth2provider"
)

// @Summary Get OAuth2 applications.
// @ID get-oauth2-applications
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param user_id query string false "Filter by applications authorized for a user"
// @Success 200 {array} codersdk.OAuth2ProviderApp
// @Router /oauth2-provider/apps [get]
func (api *API) oAuth2ProviderApps() http.HandlerFunc {
	return oauth2provider.ListApps(api.Database, api.AccessURL)
}

// @Summary Get OAuth2 application.
// @ID get-oauth2-application
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param app path string true "App ID"
// @Success 200 {object} codersdk.OAuth2ProviderApp
// @Router /oauth2-provider/apps/{app} [get]
func (api *API) oAuth2ProviderApp() http.HandlerFunc {
	return oauth2provider.GetApp(api.AccessURL)
}

// @Summary Create OAuth2 application.
// @ID create-oauth2-application
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.PostOAuth2ProviderAppRequest true "The OAuth2 application to create."
// @Success 200 {object} codersdk.OAuth2ProviderApp
// @Router /oauth2-provider/apps [post]
func (api *API) postOAuth2ProviderApp() http.HandlerFunc {
	return oauth2provider.CreateApp(api.Database, api.AccessURL, api.Auditor.Load(), api.Logger)
}

// @Summary Update OAuth2 application.
// @ID update-oauth2-application
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param app path string true "App ID"
// @Param request body codersdk.PutOAuth2ProviderAppRequest true "Update an OAuth2 application."
// @Success 200 {object} codersdk.OAuth2ProviderApp
// @Router /oauth2-provider/apps/{app} [put]
func (api *API) putOAuth2ProviderApp() http.HandlerFunc {
	return oauth2provider.UpdateApp(api.Database, api.AccessURL, api.Auditor.Load(), api.Logger)
}

// @Summary Delete OAuth2 application.
// @ID delete-oauth2-application
// @Security CoderSessionToken
// @Tags Enterprise
// @Param app path string true "App ID"
// @Success 204
// @Router /oauth2-provider/apps/{app} [delete]
func (api *API) deleteOAuth2ProviderApp() http.HandlerFunc {
	return oauth2provider.DeleteApp(api.Database, api.Auditor.Load(), api.Logger)
}

// @Summary Get OAuth2 application secrets.
// @ID get-oauth2-application-secrets
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param app path string true "App ID"
// @Success 200 {array} codersdk.OAuth2ProviderAppSecret
// @Router /oauth2-provider/apps/{app}/secrets [get]
func (api *API) oAuth2ProviderAppSecrets() http.HandlerFunc {
	return oauth2provider.GetAppSecrets(api.Database)
}

// @Summary Create OAuth2 application secret.
// @ID create-oauth2-application-secret
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param app path string true "App ID"
// @Success 200 {array} codersdk.OAuth2ProviderAppSecretFull
// @Router /oauth2-provider/apps/{app}/secrets [post]
func (api *API) postOAuth2ProviderAppSecret() http.HandlerFunc {
	return oauth2provider.CreateAppSecret(api.Database, api.Auditor.Load(), api.Logger)
}

// @Summary Delete OAuth2 application secret.
// @ID delete-oauth2-application-secret
// @Security CoderSessionToken
// @Tags Enterprise
// @Param app path string true "App ID"
// @Param secretID path string true "Secret ID"
// @Success 204
// @Router /oauth2-provider/apps/{app}/secrets/{secretID} [delete]
func (api *API) deleteOAuth2ProviderAppSecret() http.HandlerFunc {
	return oauth2provider.DeleteAppSecret(api.Database, api.Auditor.Load(), api.Logger)
}

// @Summary OAuth2 authorization request (GET - show authorization page).
// @ID oauth2-authorization-request-get
// @Security CoderSessionToken
// @Tags Enterprise
// @Param client_id query string true "Client ID"
// @Param state query string true "A random unguessable string"
// @Param response_type query codersdk.OAuth2ProviderResponseType true "Response type"
// @Param redirect_uri query string false "Redirect here after authorization"
// @Param scope query string false "Token scopes (currently ignored)"
// @Success 200 "Returns HTML authorization page"
// @Router /oauth2/authorize [get]
func (api *API) getOAuth2ProviderAppAuthorize() http.HandlerFunc {
	return oauth2provider.ShowAuthorizePage(api.AccessURL)
}

// @Summary OAuth2 authorization request (POST - process authorization).
// @ID oauth2-authorization-request-post
// @Security CoderSessionToken
// @Tags Enterprise
// @Param client_id query string true "Client ID"
// @Param state query string true "A random unguessable string"
// @Param response_type query codersdk.OAuth2ProviderResponseType true "Response type"
// @Param redirect_uri query string false "Redirect here after authorization"
// @Param scope query string false "Token scopes (currently ignored)"
// @Success 302 "Returns redirect with authorization code"
// @Router /oauth2/authorize [post]
func (api *API) postOAuth2ProviderAppAuthorize() http.HandlerFunc {
	return oauth2provider.ProcessAuthorize(api.Database)
}

// @Summary OAuth2 token exchange.
// @ID oauth2-token-exchange
// @Produce json
// @Tags Enterprise
// @Param client_id formData string false "Client ID, required if grant_type=authorization_code"
// @Param client_secret formData string false "Client secret, required if grant_type=authorization_code"
// @Param code formData string false "Authorization code, required if grant_type=authorization_code"
// @Param refresh_token formData string false "Refresh token, required if grant_type=refresh_token"
// @Param grant_type formData codersdk.OAuth2ProviderGrantType true "Grant type"
// @Success 200 {object} oauth2.Token
// @Router /oauth2/tokens [post]
func (api *API) postOAuth2ProviderAppToken() http.HandlerFunc {
	return oauth2provider.Tokens(api.Database, api.DeploymentValues.Sessions)
}

// @Summary Delete OAuth2 application tokens.
// @ID delete-oauth2-application-tokens
// @Security CoderSessionToken
// @Tags Enterprise
// @Param client_id query string true "Client ID"
// @Success 204
// @Router /oauth2/tokens [delete]
func (api *API) deleteOAuth2ProviderAppTokens() http.HandlerFunc {
	return oauth2provider.RevokeApp(api.Database)
}

// @Summary Revoke OAuth2 tokens (RFC 7009).
// @ID oauth2-token-revocation
// @Accept x-www-form-urlencoded
// @Tags Enterprise
// @Param client_id formData string true "Client ID for authentication"
// @Param token formData string true "The token to revoke"
// @Param token_type_hint formData string false "Hint about token type (access_token or refresh_token)"
// @Success 200 "Token successfully revoked"
// @Router /oauth2/revoke [post]
func (api *API) revokeOAuth2Token() http.HandlerFunc {
	return oauth2provider.RevokeToken(api.Database, api.Logger)
}

// @Summary OAuth2 authorization server metadata.
// @ID oauth2-authorization-server-metadata
// @Produce json
// @Tags Enterprise
// @Success 200 {object} codersdk.OAuth2AuthorizationServerMetadata
// @Router /.well-known/oauth-authorization-server [get]
func (api *API) oauth2AuthorizationServerMetadata() http.HandlerFunc {
	return oauth2provider.GetAuthorizationServerMetadata(api.AccessURL)
}

// @Summary OAuth2 protected resource metadata.
// @ID oauth2-protected-resource-metadata
// @Produce json
// @Tags Enterprise
// @Success 200 {object} codersdk.OAuth2ProtectedResourceMetadata
// @Router /.well-known/oauth-protected-resource [get]
func (api *API) oauth2ProtectedResourceMetadata() http.HandlerFunc {
	return oauth2provider.GetProtectedResourceMetadata(api.AccessURL)
}

// @Summary OAuth2 dynamic client registration (RFC 7591)
// @ID oauth2-dynamic-client-registration
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.OAuth2ClientRegistrationRequest true "Client registration request"
// @Success 201 {object} codersdk.OAuth2ClientRegistrationResponse
// @Router /oauth2/register [post]
func (api *API) postOAuth2ClientRegistration() http.HandlerFunc {
	return oauth2provider.CreateDynamicClientRegistration(api.Database, api.AccessURL, api.Auditor.Load(), api.Logger)
}

// @Summary Get OAuth2 client configuration (RFC 7592)
// @ID get-oauth2-client-configuration
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param client_id path string true "Client ID"
// @Success 200 {object} codersdk.OAuth2ClientConfiguration
// @Router /oauth2/clients/{client_id} [get]
func (api *API) oauth2ClientConfiguration() http.HandlerFunc {
	return oauth2provider.GetClientConfiguration(api.Database)
}

// @Summary Update OAuth2 client configuration (RFC 7592)
// @ID put-oauth2-client-configuration
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param client_id path string true "Client ID"
// @Param request body codersdk.OAuth2ClientRegistrationRequest true "Client update request"
// @Success 200 {object} codersdk.OAuth2ClientConfiguration
// @Router /oauth2/clients/{client_id} [put]
func (api *API) putOAuth2ClientConfiguration() http.HandlerFunc {
	return oauth2provider.UpdateClientConfiguration(api.Database, api.Auditor.Load(), api.Logger)
}

// @Summary Delete OAuth2 client registration (RFC 7592)
// @ID delete-oauth2-client-configuration
// @Tags Enterprise
// @Param client_id path string true "Client ID"
// @Success 204
// @Router /oauth2/clients/{client_id} [delete]
func (api *API) deleteOAuth2ClientConfiguration() http.HandlerFunc {
	return oauth2provider.DeleteClientConfiguration(api.Database, api.Auditor.Load(), api.Logger)
}
