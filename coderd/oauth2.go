package coderd

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/oauth2provider"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// @Summary Get OAuth2 applications.
// @Description Returns the OAuth2 applications registered on this deployment, or only the ones a user has authorized when you set `user_id`.
// @Description Any member can list applications, but listing another user's authorizations requires the Owner role.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID get-oauth2-applications
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param user_id query string false "ID of a user. Returns only the applications that user has authorized, instead of all applications. Must be a UUID, so the `me` shortcut does not work."
// @Success 200 {array} codersdk.OAuth2ProviderApp
// @Router /api/v2/oauth2-provider/apps [get]
func (api *API) oAuth2ProviderApps() http.HandlerFunc {
	return oauth2provider.ListApps(api.Database, api.AccessURL)
}

// @Summary Get OAuth2 application.
// @Description Returns one OAuth2 application, including its redirect URIs and scope allowlist.
// @Description Any member can read an application.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID get-oauth2-application
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param app path string true "OAuth2 application ID."
// @Success 200 {object} codersdk.OAuth2ProviderApp
// @Router /api/v2/oauth2-provider/apps/{app} [get]
func (api *API) oAuth2ProviderApp() http.HandlerFunc {
	return oauth2provider.GetApp(api.AccessURL)
}

// @Summary Create OAuth2 application.
// @Description Registers an OAuth2 application that can use Coder as its authorization server.
// @Description Requires permission to create OAuth2 applications, which the Owner role has.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID create-oauth2-application
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.PostOAuth2ProviderAppRequest true "The OAuth2 application to create."
// @Success 200 {object} codersdk.OAuth2ProviderApp
// @Router /api/v2/oauth2-provider/apps [post]
func (api *API) postOAuth2ProviderApp() http.HandlerFunc {
	return oauth2provider.CreateApp(api.Database, api.AccessURL, api.Auditor.Load(), api.Logger)
}

// @Summary Update OAuth2 application.
// @Description Updates an OAuth2 application's name, icon, and redirect URIs, and its scope allowlist when you send `scope`.
// @Description Requires permission to update OAuth2 applications, which the Owner role has.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID update-oauth2-application
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param app path string true "OAuth2 application ID."
// @Param request body codersdk.PutOAuth2ProviderAppRequest true "New settings for the OAuth2 application. Omit both `redirect_uris` and `callback_url` to keep the stored redirect URIs, and omit `scope` to keep the scope allowlist. Omitting `icon` clears it."
// @Success 200 {object} codersdk.OAuth2ProviderApp
// @Router /api/v2/oauth2-provider/apps/{app} [put]
func (api *API) putOAuth2ProviderApp() http.HandlerFunc {
	return oauth2provider.UpdateApp(api.Database, api.AccessURL, api.Auditor.Load(), api.Logger)
}

// @Summary Delete OAuth2 application.
// @Description Deletes an OAuth2 application along with its secrets and authorization codes, so the application can no longer authenticate.
// @Description Requires permission to delete OAuth2 applications, which the Owner role has.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID delete-oauth2-application
// @Security CoderSessionToken
// @Tags Enterprise
// @Param app path string true "OAuth2 application ID."
// @Success 204
// @Router /api/v2/oauth2-provider/apps/{app} [delete]
func (api *API) deleteOAuth2ProviderApp() http.HandlerFunc {
	return oauth2provider.DeleteApp(api.Database, api.Auditor.Load(), api.Logger)
}

// @Summary Get OAuth2 application secrets.
// @Description Returns the secrets of an OAuth2 application, each with its ID, last-used time, and last 6 characters, but never the full secret.
// @Description Requires permission to read OAuth2 application secrets, which the Owner role has.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID get-oauth2-application-secrets
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param app path string true "OAuth2 application ID."
// @Success 200 {array} codersdk.OAuth2ProviderAppSecret
// @Router /api/v2/oauth2-provider/apps/{app}/secrets [get]
func (api *API) oAuth2ProviderAppSecrets() http.HandlerFunc {
	return oauth2provider.GetAppSecrets(api.Database)
}

// @Summary Create OAuth2 application secret.
// @Description Creates a client secret for a confidential OAuth2 application and returns the full secret once, so store it when you receive it.
// @Description Requires permission to create OAuth2 application secrets, which the Owner role has, and returns `400` for a public client, which has no secret.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID create-oauth2-application-secret
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param app path string true "OAuth2 application ID."
// @Success 200 {array} codersdk.OAuth2ProviderAppSecretFull
// @Failure 400 {object} codersdk.Response "Public clients cannot have secrets"
// @Router /api/v2/oauth2-provider/apps/{app}/secrets [post]
func (api *API) postOAuth2ProviderAppSecret() http.HandlerFunc {
	return oauth2provider.CreateAppSecret(api.Database, api.Auditor.Load(), api.Logger)
}

// @Summary Delete OAuth2 application secret.
// @Description Deletes one secret of an OAuth2 application, so the application can no longer authenticate with it.
// @Description Requires permission to delete OAuth2 application secrets, which the Owner role has.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID delete-oauth2-application-secret
// @Security CoderSessionToken
// @Tags Enterprise
// @Param app path string true "OAuth2 application ID."
// @Param secretID path string true "ID of the OAuth2 application secret."
// @Success 204
// @Router /api/v2/oauth2-provider/apps/{app}/secrets/{secretID} [delete]
func (api *API) deleteOAuth2ProviderAppSecret() http.HandlerFunc {
	return oauth2provider.DeleteAppSecret(api.Database, api.Auditor.Load(), api.Logger)
}

// @Summary OAuth2 authorization request (GET - show authorization page).
// @Description Shows the consent page where a signed-in user approves an OAuth2 client's request for access to their Coder account (RFC 6749 section 4.1.1).
// @Description The OAuth2 client sends the user's browser here, and a browser without a session goes to the login page first.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID oauth2-authorization-request-get
// @Security CoderSessionToken
// @Tags Enterprise
// @Param client_id query string true "Client ID of the requesting OAuth2 client, which is the ID of its application."
// @Param state query string false "A random unguessable string, echoed back on the callback"
// @Param response_type query string true "Response type. Only `code` is supported." Enums(code)
// @Param redirect_uri query string false "Redirect here after authorization"
// @Param scope query string false "Space-separated scopes to request. Each must be supported by this deployment, and the app's allowlist, when it has one, must cover the permissions requested rather than name each scope. Defaults to that allowlist, or to coder:all for an app with no allowlist"
// @Param code_challenge query string true "PKCE code challenge, 43 to 128 characters from [A-Za-z0-9-._~] (RFC 7636)"
// @Param code_challenge_method query string false "PKCE challenge method. S256 only; omitting it means S256" Enums(S256)
// @Param resource query string false "RFC 8707 resource indicator: an absolute URI without a fragment"
// @Success 200 "Returns HTML authorization page"
// @Success 302 "Redirects to the app's registered callback carrying an OAuth2 error (RFC 6749 4.1.2.1)"
// @Failure 400 "HTML error page. The failure names the redirect URI or the client, so RFC 6749 4.1.2.1 withholds the callback"
// @Failure 500 "HTML error page. One of the app's registered redirect URIs is not usable"
// @Router /oauth2/authorize [get]
func (api *API) getOAuth2ProviderAppAuthorize() http.HandlerFunc {
	return oauth2provider.ShowAuthorizePage(api.AccessURL, api.Logger)
}

// @Summary OAuth2 authorization request (POST - process authorization).
// @Description Records the user's approval and redirects the browser to the client's registered callback with a single-use authorization code that expires after 10 minutes (RFC 6749 section 4.1.2).
// @Description The consent page submits the same query parameters as the `GET` request, and the route needs the user's session.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID oauth2-authorization-request-post
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param client_id query string true "Client ID of the requesting OAuth2 client, which is the ID of its application."
// @Param state query string false "A random unguessable string, echoed back on the callback"
// @Param response_type query string true "Response type. Only `code` is supported." Enums(code)
// @Param redirect_uri query string false "Redirect here after authorization"
// @Param scope query string false "Space-separated scopes to request. Each must be supported by this deployment, and the app's allowlist, when it has one, must cover the permissions requested rather than name each scope. Defaults to that allowlist, or to coder:all for an app with no allowlist"
// @Param code_challenge query string true "PKCE code challenge, 43 to 128 characters from [A-Za-z0-9-._~] (RFC 7636)"
// @Param code_challenge_method query string false "PKCE challenge method. S256 only; omitting it means S256" Enums(S256)
// @Param resource query string false "RFC 8707 resource indicator: an absolute URI without a fragment"
// @Success 302 "Redirects to the app's registered callback carrying either an authorization code or an OAuth2 error (RFC 6749 4.1.2.1)"
// @Failure 400 {object} codersdk.OAuth2Error "The failure names the redirect URI or the client, so RFC 6749 4.1.2.1 withholds the callback"
// @Failure 500 {object} codersdk.OAuth2Error "One of the app's registered redirect URIs is not usable"
// @Router /oauth2/authorize [post]
func (api *API) postOAuth2ProviderAppAuthorize() http.HandlerFunc {
	return oauth2provider.ProcessAuthorize(api.Database, api.Logger)
}

// @Summary OAuth2 token exchange.
// @Description Exchanges an authorization code or a refresh token for an access token and a new refresh token (RFC 6749 sections 4.1.3 and 6).
// @Description Doesn't require a session token, because the OAuth2 client authenticates with its client credentials, as form fields or HTTP Basic, or with `code_verifier` when it is a public client.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID oauth2-token-exchange
// @Produce json
// @Tags Enterprise
// @Param client_id formData string false "Client ID, required if grant_type=authorization_code"
// @Param client_secret formData string false "Client secret, required if grant_type=authorization_code and the client is confidential. Public clients (token_endpoint_auth_method=none) send no secret."
// @Param code formData string false "Authorization code, required if grant_type=authorization_code"
// @Param code_verifier formData string false "PKCE code verifier, required if grant_type=authorization_code. 43-128 characters per RFC 7636."
// @Param refresh_token formData string false "Refresh token, required if grant_type=refresh_token"
// @Param grant_type formData codersdk.OAuth2ProviderGrantType true "Grant type. Use `authorization_code` with `code`, or `refresh_token` with `refresh_token`. Other values return `unsupported_grant_type`."
// @Success 200 {object} codersdk.OAuth2TokenResponse
// @Failure 400 {object} codersdk.OAuth2Error "invalid_request: client_secret in the URL query string, or a missing or malformed parameter"
// @Failure 413 {object} codersdk.OAuth2Error "Request body exceeds 4 MiB"
// @Router /oauth2/tokens [post]
func (api *API) postOAuth2ProviderAppToken() http.HandlerFunc {
	return oauth2provider.Tokens(api.Database, api.DeploymentValues.Sessions, api.Logger)
}

// @Summary Delete OAuth2 application tokens.
// @Description Revokes the signed-in user's authorization codes and tokens for one OAuth2 application, so the application loses that user's access.
// @Description Authenticates with the user's session token instead of client credentials, so to revoke one token as the OAuth2 client, use `POST /oauth2/revoke`.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID delete-oauth2-application-tokens
// @Security CoderSessionToken
// @Tags Enterprise
// @Param client_id query string true "Client ID of the OAuth2 application, which is the application ID."
// @Success 204
// @Router /oauth2/tokens [delete]
func (api *API) deleteOAuth2ProviderAppTokens() http.HandlerFunc {
	return oauth2provider.RevokeApp(api.Database)
}

// @Summary Revoke OAuth2 tokens (RFC 7009).
// @Description Revokes an access token or a refresh token that was issued to the calling OAuth2 client (RFC 7009).
// @Description Doesn't require a session token, because the OAuth2 client authenticates with its client credentials, as form fields or HTTP Basic, and a `200` response doesn't confirm that the token existed or belonged to the client.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID oauth2-token-revocation
// @Accept x-www-form-urlencoded
// @Produce json
// @Tags Enterprise
// @Param Authorization header string false "HTTP Basic credentials, the client_id as the username and the client_secret as the password. A confidential client sends these or the form fields below, not both." example(Basic Y2xpZW50X2lkOmNsaWVudF9zZWNyZXQ=)
// @Param client_id formData string false "Client ID, required unless sent as the HTTP Basic username"
// @Param client_secret formData string false "Client secret, required for a confidential client unless sent as the HTTP Basic password. Public clients (token_endpoint_auth_method=none) send no secret."
// @Param token formData string true "The token to revoke"
// @Param token_type_hint formData string false "Hint about token type (access_token or refresh_token)"
// @Success 200 "Token successfully revoked. A 200 does not confirm that the token existed or belonged to the client"
// @Failure 400 {object} codersdk.OAuth2Error "invalid_request: a missing client_id or token, credentials in both the Authorization header and the body, client_secret in the URL query string, or a malformed token"
// @Failure 401 {object} codersdk.OAuth2Error "invalid_client: the client is unknown, or a confidential client did not present a valid secret"
// @Failure 413 {object} codersdk.OAuth2Error "Request body exceeds 4 MiB"
// @Router /oauth2/revoke [post]
func (api *API) revokeOAuth2Token() http.HandlerFunc {
	return oauth2provider.RevokeToken(api.Database, api.Logger)
}

// @Summary OAuth2 authorization server metadata.
// @Description Returns the metadata that OAuth2 clients use to discover Coder's OAuth2 endpoints, supported grants, and scopes (RFC 8414).
// @Description Doesn't require a session token, and includes `registration_endpoint` only when dynamic client registration is enabled.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID oauth2-authorization-server-metadata
// @Produce json
// @Tags Enterprise
// @Success 200 {object} codersdk.OAuth2AuthorizationServerMetadata
// @Router /.well-known/oauth-authorization-server [get]
func (api *API) oauth2AuthorizationServerMetadata() http.HandlerFunc {
	return oauth2provider.GetAuthorizationServerMetadata(api.Database, api.AccessURL)
}

// @Summary OAuth2 protected resource metadata.
// @Description Returns the metadata that tells an OAuth2 client which authorization server protects this deployment and which scopes it supports (RFC 9728).
// @Description Doesn't require a session token, and answers only for the deployment root and the Coder MCP endpoint's path.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID oauth2-protected-resource-metadata
// @Produce json
// @Tags Enterprise
// @Success 200 {object} codersdk.OAuth2ProtectedResourceMetadata
// @Router /.well-known/oauth-protected-resource [get]
func (api *API) oauth2ProtectedResourceMetadata() http.HandlerFunc {
	return oauth2provider.GetProtectedResourceMetadata(api.AccessURL)
}

// @Summary OAuth2 dynamic client registration (RFC 7591)
// @Description Registers a new OAuth2 client and returns its `client_id`, its `client_secret` when it is confidential, and a registration access token you use to manage the client later (RFC 7591).
// @Description Doesn't require a session token, but returns `403` unless an Owner has enabled dynamic client registration in the OAuth2 provider settings.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID oauth2-dynamic-client-registration
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.OAuth2ClientRegistrationRequest true "Client metadata to register, as defined by RFC 7591. Requires `redirect_uris`."
// @Success 201 {object} codersdk.OAuth2ClientRegistrationResponse
// @Failure 413 {object} codersdk.OAuth2Error "Request body exceeds 4 MiB"
// @Router /oauth2/register [post]
func (api *API) postOAuth2ClientRegistration() http.HandlerFunc {
	return oauth2provider.CreateDynamicClientRegistration(api.Database, api.AccessURL, api.Auditor.Load(), api.Logger)
}

// @Summary Get OAuth2 provider settings.
// @Description Returns the OAuth2 provider settings for the deployment, such as whether dynamic client registration is enabled.
// @Description Requires permission to read the deployment configuration, which the Owner and Auditor roles have.
// @Description Works while the OAuth2 provider is disabled.
// @ID get-oauth2-provider-settings
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Success 200 {object} codersdk.OAuth2ProviderSettings
// @Router /api/v2/oauth2-provider/settings [get]
func (api *API) oauth2ProviderSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	enabled, err := api.Database.GetOAuth2DCREnabled(ctx)
	if err != nil {
		if rbac.IsUnauthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.OAuth2ProviderSettings{
		DynamicClientRegistrationEnabled: ptr.Ref(enabled),
	})
}

// @Summary Update OAuth2 provider settings.
// @Description Updates the OAuth2 provider settings for the deployment, such as whether `POST /oauth2/register` accepts new clients, and returns the resulting values.
// @Description Requires permission to update the deployment configuration, which the Owner role has.
// @Description Works while the OAuth2 provider is disabled.
// @ID update-oauth2-provider-settings
// @Security CoderSessionToken
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.OAuth2ProviderSettings true "OAuth2 provider settings to change. Omitted fields keep their current value."
// @Success 200 {object} codersdk.OAuth2ProviderSettings
// @Router /api/v2/oauth2-provider/settings [put]
func (api *API) putOAuth2ProviderSettings(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req codersdk.OAuth2ProviderSettings
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	aReq, commitAudit := audit.InitRequest[database.OAuth2ProviderSettings](rw, &audit.RequestParams{
		Audit:   *api.Auditor.Load(),
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit()

	var resolvedEnabled bool
	err := api.Database.InTx(func(tx database.Store) error {
		oldEnabled, err := tx.GetOAuth2DCREnabled(ctx)
		if err != nil {
			return err
		}
		aReq.Old = database.OAuth2ProviderSettings{
			DynamicClientRegistrationEnabled: oldEnabled,
		}

		// A nil field means the caller omitted it, leave the current value
		// alone rather than overwrite it with a decoded zero value. This
		// matters once a second field lands in this struct: an older client
		// that only knows about this field must not silently clear a newer
		// one it never sent.
		resolvedEnabled = oldEnabled
		if req.DynamicClientRegistrationEnabled != nil {
			resolvedEnabled = *req.DynamicClientRegistrationEnabled
			if err := tx.UpsertOAuth2DCREnabled(ctx, resolvedEnabled); err != nil {
				return err
			}
		}
		aReq.New = database.OAuth2ProviderSettings{
			ID:                               uuid.New(),
			DynamicClientRegistrationEnabled: resolvedEnabled,
		}
		return nil
	}, &database.TxOptions{TxIdentifier: "update_oauth2_provider_settings"})
	if err != nil {
		if rbac.IsUnauthorizedError(err) {
			httpapi.Forbidden(rw)
			return
		}
		httpapi.InternalServerError(rw, err)
		return
	}
	httpapi.Write(ctx, rw, http.StatusOK, codersdk.OAuth2ProviderSettings{
		DynamicClientRegistrationEnabled: ptr.Ref(resolvedEnabled),
	})
}

// @Summary Get OAuth2 client configuration (RFC 7592)
// @Description Returns the registered configuration of a dynamically registered OAuth2 client, without its secret (RFC 7592).
// @Description Authenticates with the registration access token from `POST /oauth2/register`, sent as a bearer token, and doesn't require a session token.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID get-oauth2-client-configuration
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param client_id path string true "Client ID that `POST /oauth2/register` returned."
// @Success 200 {object} codersdk.OAuth2ClientConfiguration
// @Router /oauth2/clients/{client_id} [get]
func (api *API) oauth2ClientConfiguration() http.HandlerFunc {
	return oauth2provider.GetClientConfiguration(api.Database)
}

// @Summary Update OAuth2 client configuration (RFC 7592)
// @Description Replaces the registered metadata of a dynamically registered OAuth2 client (RFC 7592).
// @Description Authenticates with the registration access token from `POST /oauth2/register`, sent as a bearer token, and doesn't require a session token.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID put-oauth2-client-configuration
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param client_id path string true "Client ID that `POST /oauth2/register` returned."
// @Param request body codersdk.OAuth2ClientRegistrationRequest true "New client metadata. It replaces the registered metadata, using the same fields as the registration request."
// @Success 200 {object} codersdk.OAuth2ClientConfiguration
// @Failure 413 {object} codersdk.OAuth2Error "Request body exceeds 4 MiB"
// @Router /oauth2/clients/{client_id} [put]
func (api *API) putOAuth2ClientConfiguration() http.HandlerFunc {
	return oauth2provider.UpdateClientConfiguration(api.Database, api.Auditor.Load(), api.Logger)
}

// @Summary Delete OAuth2 client registration (RFC 7592)
// @Description Deletes a dynamically registered OAuth2 client along with its secrets and authorization codes (RFC 7592).
// @Description Authenticates with the registration access token from `POST /oauth2/register`, sent as a bearer token, and doesn't require a session token.
// @Description Returns `404` unless the OAuth2 provider is enabled with `--oauth2-provider-enable`.
// @ID delete-oauth2-client-configuration
// @Tags Enterprise
// @Param client_id path string true "Client ID that `POST /oauth2/register` returned."
// @Success 204
// @Router /oauth2/clients/{client_id} [delete]
func (api *API) deleteOAuth2ClientConfiguration() http.HandlerFunc {
	return oauth2provider.DeleteClientConfiguration(api.Database, api.Auditor.Load(), api.Logger)
}
