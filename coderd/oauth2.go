package coderd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/sqlc-dev/pqtype"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/identityprovider"
	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)

// Constants for OAuth2 secret generation (RFC 7591)
const (
	secretLength        = 40 // Length of the actual secret part
	secretPrefixLength  = 10 // Length of the prefix for database lookup
	displaySecretLength = 6  // Length of visible part in UI (last 6 characters)
)

func (*API) oAuth2ProviderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if !buildinfo.IsDev() {
			httpapi.Write(r.Context(), rw, http.StatusForbidden, codersdk.Response{
				Message: "OAuth2 provider is under development.",
			})
			return
		}

		next.ServeHTTP(rw, r)
	})
}

// @Summary Get OAuth2 applications.
// @ID get-oauth2-applications
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param user_id query string false "Filter by applications authorized for a user"
// @Success 200 {array} codersdk.OAuth2ProviderApp
// @Router /oauth2-provider/apps [get]
func (api *API) oAuth2ProviderApps(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rawUserID := r.URL.Query().Get("user_id")
	if rawUserID == "" {
		dbApps, err := api.Database.GetOAuth2ProviderApps(ctx)
		if err != nil {
			httpapi.InternalServerError(rw, err)
			return
		}
		httpapi.Write(ctx, rw, http.StatusOK, db2sdk.OAuth2ProviderApps(api.AccessURL, dbApps))
		return
	}

	userID, err := uuid.Parse(rawUserID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid user UUID",
			Detail:  fmt.Sprintf("queried user_id=%q", userID),
		})
		return
	}

	userApps, err := api.Database.GetOAuth2ProviderAppsByUserID(ctx, userID)
	if err != nil {
		httpapi.InternalServerError(rw, err)
		return
	}

	var sdkApps []codersdk.OAuth2ProviderApp
	for _, app := range userApps {
		sdkApps = append(sdkApps, db2sdk.OAuth2ProviderApp(api.AccessURL, app.OAuth2ProviderApp))
	}
	httpapi.Write(ctx, rw, http.StatusOK, sdkApps)
}

// @Summary Get OAuth2 application.
// @ID get-oauth2-application
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param app path string true "App ID"
// @Success 200 {object} codersdk.OAuth2ProviderApp
// @Router /oauth2-provider/apps/{app} [get]
func (api *API) oAuth2ProviderApp(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app := httpmw.OAuth2ProviderApp(r)
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.OAuth2ProviderApp(api.AccessURL, app))
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
func (api *API) postOAuth2ProviderApp(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.OAuth2ProviderApp](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()
	var req codersdk.PostOAuth2ProviderAppRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	app, err := api.Database.InsertOAuth2ProviderApp(ctx, database.InsertOAuth2ProviderAppParams{
		ID:                      uuid.New(),
		CreatedAt:               dbtime.Now(),
		UpdatedAt:               dbtime.Now(),
		Name:                    req.Name,
		Icon:                    req.Icon,
		CallbackURL:             req.CallbackURL,
		RedirectUris:            []string{},
		ClientType:              sql.NullString{String: "confidential", Valid: true},
		DynamicallyRegistered:   sql.NullBool{Bool: false, Valid: true},
		ClientIDIssuedAt:        sql.NullTime{},
		ClientSecretExpiresAt:   sql.NullTime{},
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: sql.NullString{String: "client_secret_post", Valid: true},
		Scope:                   sql.NullString{},
		Contacts:                []string{},
		ClientUri:               sql.NullString{},
		LogoUri:                 sql.NullString{},
		TosUri:                  sql.NullString{},
		PolicyUri:               sql.NullString{},
		JwksUri:                 sql.NullString{},
		Jwks:                    pqtype.NullRawMessage{},
		SoftwareID:              sql.NullString{},
		SoftwareVersion:         sql.NullString{},
		RegistrationAccessToken: sql.NullString{},
		RegistrationClientUri:   sql.NullString{},
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating OAuth2 application.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = app
	httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.OAuth2ProviderApp(api.AccessURL, app))
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
func (api *API) putOAuth2ProviderApp(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		app               = httpmw.OAuth2ProviderApp(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.OAuth2ProviderApp](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionWrite,
		})
	)
	aReq.Old = app
	defer commitAudit()
	var req codersdk.PutOAuth2ProviderAppRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}
	app, err := api.Database.UpdateOAuth2ProviderAppByID(ctx, database.UpdateOAuth2ProviderAppByIDParams{
		ID:                      app.ID,
		UpdatedAt:               dbtime.Now(),
		Name:                    req.Name,
		Icon:                    req.Icon,
		CallbackURL:             req.CallbackURL,
		RedirectUris:            app.RedirectUris,            // Keep existing value
		ClientType:              app.ClientType,              // Keep existing value
		DynamicallyRegistered:   app.DynamicallyRegistered,   // Keep existing value
		ClientSecretExpiresAt:   app.ClientSecretExpiresAt,   // Keep existing value
		GrantTypes:              app.GrantTypes,              // Keep existing value
		ResponseTypes:           app.ResponseTypes,           // Keep existing value
		TokenEndpointAuthMethod: app.TokenEndpointAuthMethod, // Keep existing value
		Scope:                   app.Scope,                   // Keep existing value
		Contacts:                app.Contacts,                // Keep existing value
		ClientUri:               app.ClientUri,               // Keep existing value
		LogoUri:                 app.LogoUri,                 // Keep existing value
		TosUri:                  app.TosUri,                  // Keep existing value
		PolicyUri:               app.PolicyUri,               // Keep existing value
		JwksUri:                 app.JwksUri,                 // Keep existing value
		Jwks:                    app.Jwks,                    // Keep existing value
		SoftwareID:              app.SoftwareID,              // Keep existing value
		SoftwareVersion:         app.SoftwareVersion,         // Keep existing value
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error updating OAuth2 application.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = app
	httpapi.Write(ctx, rw, http.StatusOK, db2sdk.OAuth2ProviderApp(api.AccessURL, app))
}

// @Summary Delete OAuth2 application.
// @ID delete-oauth2-application
// @Security CoderSessionToken
// @Tags Enterprise
// @Param app path string true "App ID"
// @Success 204
// @Router /oauth2-provider/apps/{app} [delete]
func (api *API) deleteOAuth2ProviderApp(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		app               = httpmw.OAuth2ProviderApp(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.OAuth2ProviderApp](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
	)
	aReq.Old = app
	defer commitAudit()
	err := api.Database.DeleteOAuth2ProviderAppByID(ctx, app.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting OAuth2 application.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// @Summary Get OAuth2 application secrets.
// @ID get-oauth2-application-secrets
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param app path string true "App ID"
// @Success 200 {array} codersdk.OAuth2ProviderAppSecret
// @Router /oauth2-provider/apps/{app}/secrets [get]
func (api *API) oAuth2ProviderAppSecrets(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app := httpmw.OAuth2ProviderApp(r)
	dbSecrets, err := api.Database.GetOAuth2ProviderAppSecretsByAppID(ctx, app.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error getting OAuth2 client secrets.",
			Detail:  err.Error(),
		})
		return
	}
	secrets := []codersdk.OAuth2ProviderAppSecret{}
	for _, secret := range dbSecrets {
		secrets = append(secrets, codersdk.OAuth2ProviderAppSecret{
			ID:                    secret.ID,
			LastUsedAt:            codersdk.NullTime{NullTime: secret.LastUsedAt},
			ClientSecretTruncated: secret.DisplaySecret,
		})
	}
	httpapi.Write(ctx, rw, http.StatusOK, secrets)
}

// @Summary Create OAuth2 application secret.
// @ID create-oauth2-application-secret
// @Security CoderSessionToken
// @Produce json
// @Tags Enterprise
// @Param app path string true "App ID"
// @Success 200 {array} codersdk.OAuth2ProviderAppSecretFull
// @Router /oauth2-provider/apps/{app}/secrets [post]
func (api *API) postOAuth2ProviderAppSecret(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		app               = httpmw.OAuth2ProviderApp(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.OAuth2ProviderAppSecret](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionCreate,
		})
	)
	defer commitAudit()
	secret, err := identityprovider.GenerateSecret()
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to generate OAuth2 client secret.",
			Detail:  err.Error(),
		})
		return
	}
	dbSecret, err := api.Database.InsertOAuth2ProviderAppSecret(ctx, database.InsertOAuth2ProviderAppSecretParams{
		ID:           uuid.New(),
		CreatedAt:    dbtime.Now(),
		SecretPrefix: []byte(secret.Prefix),
		HashedSecret: []byte(secret.Hashed),
		// DisplaySecret is the last six characters of the original unhashed secret.
		// This is done so they can be differentiated and it matches how GitHub
		// displays their client secrets.
		DisplaySecret: secret.Formatted[len(secret.Formatted)-6:],
		AppID:         app.ID,
	})
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error creating OAuth2 client secret.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = dbSecret
	httpapi.Write(ctx, rw, http.StatusCreated, codersdk.OAuth2ProviderAppSecretFull{
		ID:               dbSecret.ID,
		ClientSecretFull: secret.Formatted,
	})
}

// @Summary Delete OAuth2 application secret.
// @ID delete-oauth2-application-secret
// @Security CoderSessionToken
// @Tags Enterprise
// @Param app path string true "App ID"
// @Param secretID path string true "Secret ID"
// @Success 204
// @Router /oauth2-provider/apps/{app}/secrets/{secretID} [delete]
func (api *API) deleteOAuth2ProviderAppSecret(rw http.ResponseWriter, r *http.Request) {
	var (
		ctx               = r.Context()
		secret            = httpmw.OAuth2ProviderAppSecret(r)
		auditor           = api.Auditor.Load()
		aReq, commitAudit = audit.InitRequest[database.OAuth2ProviderAppSecret](rw, &audit.RequestParams{
			Audit:   *auditor,
			Log:     api.Logger,
			Request: r,
			Action:  database.AuditActionDelete,
		})
	)
	aReq.Old = secret
	defer commitAudit()
	err := api.Database.DeleteOAuth2ProviderAppSecretByID(ctx, secret.ID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Internal error deleting OAuth2 client secret.",
			Detail:  err.Error(),
		})
		return
	}
	rw.WriteHeader(http.StatusNoContent)
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
	return identityprovider.ShowAuthorizePage(api.Database, api.AccessURL)
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
	return identityprovider.ProcessAuthorize(api.Database, api.AccessURL)
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
	return identityprovider.Tokens(api.Database, api.DeploymentValues.Sessions)
}

// @Summary Delete OAuth2 application tokens.
// @ID delete-oauth2-application-tokens
// @Security CoderSessionToken
// @Tags Enterprise
// @Param client_id query string true "Client ID"
// @Success 204
// @Router /oauth2/tokens [delete]
func (api *API) deleteOAuth2ProviderAppTokens() http.HandlerFunc {
	return identityprovider.RevokeApp(api.Database)
}

// @Summary OAuth2 authorization server metadata.
// @ID oauth2-authorization-server-metadata
// @Produce json
// @Tags Enterprise
// @Success 200 {object} codersdk.OAuth2AuthorizationServerMetadata
// @Router /.well-known/oauth-authorization-server [get]
func (api *API) oauth2AuthorizationServerMetadata(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	metadata := codersdk.OAuth2AuthorizationServerMetadata{
		Issuer:                        api.AccessURL.String(),
		AuthorizationEndpoint:         api.AccessURL.JoinPath("/oauth2/authorize").String(),
		TokenEndpoint:                 api.AccessURL.JoinPath("/oauth2/tokens").String(),
		RegistrationEndpoint:          api.AccessURL.JoinPath("/oauth2/register").String(), // RFC 7591
		ResponseTypesSupported:        []string{"code"},
		GrantTypesSupported:           []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported: []string{"S256"},
		// TODO: Implement scope system
		ScopesSupported:                   []string{},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post"},
	}
	httpapi.Write(ctx, rw, http.StatusOK, metadata)
}

// @Summary OAuth2 protected resource metadata.
// @ID oauth2-protected-resource-metadata
// @Produce json
// @Tags Enterprise
// @Success 200 {object} codersdk.OAuth2ProtectedResourceMetadata
// @Router /.well-known/oauth-protected-resource [get]
func (api *API) oauth2ProtectedResourceMetadata(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	metadata := codersdk.OAuth2ProtectedResourceMetadata{
		Resource:             api.AccessURL.String(),
		AuthorizationServers: []string{api.AccessURL.String()},
		// TODO: Implement scope system based on RBAC permissions
		ScopesSupported: []string{},
		// RFC 6750 Bearer Token methods supported as fallback methods in api key middleware
		BearerMethodsSupported: []string{"header", "query"},
	}
	httpapi.Write(ctx, rw, http.StatusOK, metadata)
}

// @Summary OAuth2 dynamic client registration (RFC 7591)
// @ID oauth2-dynamic-client-registration
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param request body codersdk.OAuth2ClientRegistrationRequest true "Client registration request"
// @Success 201 {object} codersdk.OAuth2ClientRegistrationResponse
// @Router /oauth2/register [post]
func (api *API) postOAuth2ClientRegistration(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	auditor := *api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.OAuth2ProviderApp](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionCreate,
	})
	defer commitAudit()

	// Parse request
	var req codersdk.OAuth2ClientRegistrationRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		writeOAuth2RegistrationError(ctx, rw, http.StatusBadRequest,
			"invalid_client_metadata", err.Error())
		return
	}

	// Apply defaults
	req = req.ApplyDefaults()

	// Generate client credentials
	clientID := uuid.New()
	clientSecret, hashedSecret, err := generateClientCredentials()
	if err != nil {
		writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
			"server_error", "Failed to generate client credentials")
		return
	}

	// Generate registration access token for RFC 7592 management
	registrationToken, hashedRegToken, err := generateRegistrationAccessToken()
	if err != nil {
		writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
			"server_error", "Failed to generate registration token")
		return
	}

	// Store in database - use system context since this is a public endpoint
	now := dbtime.Now()
	//nolint:gocritic // Dynamic client registration is a public endpoint, system access required
	app, err := api.Database.InsertOAuth2ProviderApp(dbauthz.AsSystemRestricted(ctx), database.InsertOAuth2ProviderAppParams{
		ID:                      clientID,
		CreatedAt:               now,
		UpdatedAt:               now,
		Name:                    req.GenerateClientName(),
		Icon:                    req.LogoURI,
		CallbackURL:             req.RedirectURIs[0], // Primary redirect URI
		RedirectUris:            req.RedirectURIs,
		ClientType:              sql.NullString{String: req.DetermineClientType(), Valid: true},
		DynamicallyRegistered:   sql.NullBool{Bool: true, Valid: true},
		ClientIDIssuedAt:        sql.NullTime{Time: now, Valid: true},
		ClientSecretExpiresAt:   sql.NullTime{}, // No expiration for now
		GrantTypes:              req.GrantTypes,
		ResponseTypes:           req.ResponseTypes,
		TokenEndpointAuthMethod: sql.NullString{String: req.TokenEndpointAuthMethod, Valid: true},
		Scope:                   sql.NullString{String: req.Scope, Valid: true},
		Contacts:                req.Contacts,
		ClientUri:               sql.NullString{String: req.ClientURI, Valid: req.ClientURI != ""},
		LogoUri:                 sql.NullString{String: req.LogoURI, Valid: req.LogoURI != ""},
		TosUri:                  sql.NullString{String: req.TOSURI, Valid: req.TOSURI != ""},
		PolicyUri:               sql.NullString{String: req.PolicyURI, Valid: req.PolicyURI != ""},
		JwksUri:                 sql.NullString{String: req.JWKSURI, Valid: req.JWKSURI != ""},
		Jwks:                    pqtype.NullRawMessage{RawMessage: req.JWKS, Valid: len(req.JWKS) > 0},
		SoftwareID:              sql.NullString{String: req.SoftwareID, Valid: req.SoftwareID != ""},
		SoftwareVersion:         sql.NullString{String: req.SoftwareVersion, Valid: req.SoftwareVersion != ""},
		RegistrationAccessToken: sql.NullString{String: hashedRegToken, Valid: true},
		RegistrationClientUri:   sql.NullString{String: fmt.Sprintf("%s/oauth2/clients/%s", api.AccessURL.String(), clientID), Valid: true},
	})
	if err != nil {
		api.Logger.Error(ctx, "failed to store oauth2 client registration", slog.Error(err))
		writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
			"server_error", "Failed to store client registration")
		return
	}

	// Create client secret - parse the formatted secret to get components
	parsedSecret, err := parseFormattedSecret(clientSecret)
	if err != nil {
		writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
			"server_error", "Failed to parse generated secret")
		return
	}

	//nolint:gocritic // Dynamic client registration is a public endpoint, system access required
	_, err = api.Database.InsertOAuth2ProviderAppSecret(dbauthz.AsSystemRestricted(ctx), database.InsertOAuth2ProviderAppSecretParams{
		ID:            uuid.New(),
		CreatedAt:     now,
		SecretPrefix:  []byte(parsedSecret.prefix),
		HashedSecret:  []byte(hashedSecret),
		DisplaySecret: createDisplaySecret(clientSecret),
		AppID:         clientID,
	})
	if err != nil {
		writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
			"server_error", "Failed to store client secret")
		return
	}

	// Set audit log data
	aReq.New = app

	// Return response
	response := codersdk.OAuth2ClientRegistrationResponse{
		ClientID:                app.ID.String(),
		ClientSecret:            clientSecret,
		ClientIDIssuedAt:        app.ClientIDIssuedAt.Time.Unix(),
		ClientSecretExpiresAt:   0, // No expiration
		RedirectURIs:            app.RedirectUris,
		ClientName:              app.Name,
		ClientURI:               app.ClientUri.String,
		LogoURI:                 app.LogoUri.String,
		TOSURI:                  app.TosUri.String,
		PolicyURI:               app.PolicyUri.String,
		JWKSURI:                 app.JwksUri.String,
		JWKS:                    app.Jwks.RawMessage,
		SoftwareID:              app.SoftwareID.String,
		SoftwareVersion:         app.SoftwareVersion.String,
		GrantTypes:              app.GrantTypes,
		ResponseTypes:           app.ResponseTypes,
		TokenEndpointAuthMethod: app.TokenEndpointAuthMethod.String,
		Scope:                   app.Scope.String,
		Contacts:                app.Contacts,
		RegistrationAccessToken: registrationToken,
		RegistrationClientURI:   app.RegistrationClientUri.String,
	}

	httpapi.Write(ctx, rw, http.StatusCreated, response)
}

// Helper functions for RFC 7591 Dynamic Client Registration

// generateClientCredentials generates a client secret for OAuth2 apps
func generateClientCredentials() (plaintext, hashed string, err error) {
	// Use the same pattern as existing OAuth2 app secrets
	secret, err := identityprovider.GenerateSecret()
	if err != nil {
		return "", "", xerrors.Errorf("generate secret: %w", err)
	}

	return secret.Formatted, secret.Hashed, nil
}

// generateRegistrationAccessToken generates a registration access token for RFC 7592
func generateRegistrationAccessToken() (plaintext, hashed string, err error) {
	token, err := cryptorand.String(secretLength)
	if err != nil {
		return "", "", xerrors.Errorf("generate registration token: %w", err)
	}

	// Hash the token for storage
	hashedToken, err := userpassword.Hash(token)
	if err != nil {
		return "", "", xerrors.Errorf("hash registration token: %w", err)
	}

	return token, hashedToken, nil
}

// writeOAuth2RegistrationError writes RFC 7591 compliant error responses
func writeOAuth2RegistrationError(_ context.Context, rw http.ResponseWriter, status int, errorCode, description string) {
	// RFC 7591 error response format
	errorResponse := map[string]string{
		"error": errorCode,
	}
	if description != "" {
		errorResponse["error_description"] = description
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(errorResponse)
}

// parsedSecret represents the components of a formatted OAuth2 secret
type parsedSecret struct {
	prefix string
	secret string
}

// parseFormattedSecret parses a formatted secret like "coder_prefix_secret"
func parseFormattedSecret(secret string) (parsedSecret, error) {
	parts := strings.Split(secret, "_")
	if len(parts) != 3 {
		return parsedSecret{}, xerrors.Errorf("incorrect number of parts: %d", len(parts))
	}
	if parts[0] != "coder" {
		return parsedSecret{}, xerrors.Errorf("incorrect scheme: %s", parts[0])
	}
	return parsedSecret{
		prefix: parts[1],
		secret: parts[2],
	}, nil
}

// createDisplaySecret creates a display version of the secret showing only the last few characters
func createDisplaySecret(secret string) string {
	if len(secret) <= displaySecretLength {
		return secret
	}

	visiblePart := secret[len(secret)-displaySecretLength:]
	hiddenLength := len(secret) - displaySecretLength
	return strings.Repeat("*", hiddenLength) + visiblePart
}

// RFC 7592 Client Configuration Management Endpoints

// @Summary Get OAuth2 client configuration (RFC 7592)
// @ID get-oauth2-client-configuration
// @Accept json
// @Produce json
// @Tags Enterprise
// @Param client_id path string true "Client ID"
// @Success 200 {object} codersdk.OAuth2ClientConfiguration
// @Router /oauth2/clients/{client_id} [get]
func (api *API) oauth2ClientConfiguration(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract client ID from URL path
	clientIDStr := chi.URLParam(r, "client_id")
	clientID, err := uuid.Parse(clientIDStr)
	if err != nil {
		writeOAuth2RegistrationError(ctx, rw, http.StatusBadRequest,
			"invalid_client_metadata", "Invalid client ID format")
		return
	}

	// Get app by client ID
	//nolint:gocritic // RFC 7592 endpoints need system access to retrieve dynamically registered clients
	app, err := api.Database.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
				"invalid_token", "Client not found")
		} else {
			writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
				"server_error", "Failed to retrieve client")
		}
		return
	}

	// Check if client was dynamically registered
	if !app.DynamicallyRegistered.Bool {
		writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
			"invalid_token", "Client was not dynamically registered")
		return
	}

	// Return client configuration (without client_secret for security)
	response := codersdk.OAuth2ClientConfiguration{
		ClientID:                app.ID.String(),
		ClientIDIssuedAt:        app.ClientIDIssuedAt.Time.Unix(),
		ClientSecretExpiresAt:   0, // No expiration for now
		RedirectURIs:            app.RedirectUris,
		ClientName:              app.Name,
		ClientURI:               app.ClientUri.String,
		LogoURI:                 app.LogoUri.String,
		TOSURI:                  app.TosUri.String,
		PolicyURI:               app.PolicyUri.String,
		JWKSURI:                 app.JwksUri.String,
		JWKS:                    app.Jwks.RawMessage,
		SoftwareID:              app.SoftwareID.String,
		SoftwareVersion:         app.SoftwareVersion.String,
		GrantTypes:              app.GrantTypes,
		ResponseTypes:           app.ResponseTypes,
		TokenEndpointAuthMethod: app.TokenEndpointAuthMethod.String,
		Scope:                   app.Scope.String,
		Contacts:                app.Contacts,
		RegistrationAccessToken: "", // RFC 7592: Not returned in GET responses for security
		RegistrationClientURI:   app.RegistrationClientUri.String,
	}

	httpapi.Write(ctx, rw, http.StatusOK, response)
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
func (api *API) putOAuth2ClientConfiguration(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	auditor := *api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.OAuth2ProviderApp](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionWrite,
	})
	defer commitAudit()

	// Extract client ID from URL path
	clientIDStr := chi.URLParam(r, "client_id")
	clientID, err := uuid.Parse(clientIDStr)
	if err != nil {
		writeOAuth2RegistrationError(ctx, rw, http.StatusBadRequest,
			"invalid_client_metadata", "Invalid client ID format")
		return
	}

	// Parse request
	var req codersdk.OAuth2ClientRegistrationRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		writeOAuth2RegistrationError(ctx, rw, http.StatusBadRequest,
			"invalid_client_metadata", err.Error())
		return
	}

	// Apply defaults
	req = req.ApplyDefaults()

	// Get existing app to verify it exists and is dynamically registered
	//nolint:gocritic // RFC 7592 endpoints need system access to retrieve dynamically registered clients
	existingApp, err := api.Database.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)
	if err == nil {
		aReq.Old = existingApp
	}
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
				"invalid_token", "Client not found")
		} else {
			writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
				"server_error", "Failed to retrieve client")
		}
		return
	}

	// Check if client was dynamically registered
	if !existingApp.DynamicallyRegistered.Bool {
		writeOAuth2RegistrationError(ctx, rw, http.StatusForbidden,
			"invalid_token", "Client was not dynamically registered")
		return
	}

	// Update app in database
	now := dbtime.Now()
	//nolint:gocritic // RFC 7592 endpoints need system access to update dynamically registered clients
	updatedApp, err := api.Database.UpdateOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), database.UpdateOAuth2ProviderAppByClientIDParams{
		ID:                      clientID,
		UpdatedAt:               now,
		Name:                    req.GenerateClientName(),
		Icon:                    req.LogoURI,
		CallbackURL:             req.RedirectURIs[0], // Primary redirect URI
		RedirectUris:            req.RedirectURIs,
		ClientType:              sql.NullString{String: req.DetermineClientType(), Valid: true},
		ClientSecretExpiresAt:   sql.NullTime{}, // No expiration for now
		GrantTypes:              req.GrantTypes,
		ResponseTypes:           req.ResponseTypes,
		TokenEndpointAuthMethod: sql.NullString{String: req.TokenEndpointAuthMethod, Valid: true},
		Scope:                   sql.NullString{String: req.Scope, Valid: true},
		Contacts:                req.Contacts,
		ClientUri:               sql.NullString{String: req.ClientURI, Valid: req.ClientURI != ""},
		LogoUri:                 sql.NullString{String: req.LogoURI, Valid: req.LogoURI != ""},
		TosUri:                  sql.NullString{String: req.TOSURI, Valid: req.TOSURI != ""},
		PolicyUri:               sql.NullString{String: req.PolicyURI, Valid: req.PolicyURI != ""},
		JwksUri:                 sql.NullString{String: req.JWKSURI, Valid: req.JWKSURI != ""},
		Jwks:                    pqtype.NullRawMessage{RawMessage: req.JWKS, Valid: len(req.JWKS) > 0},
		SoftwareID:              sql.NullString{String: req.SoftwareID, Valid: req.SoftwareID != ""},
		SoftwareVersion:         sql.NullString{String: req.SoftwareVersion, Valid: req.SoftwareVersion != ""},
	})
	if err != nil {
		writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
			"server_error", "Failed to update client")
		return
	}

	// Set audit log data
	aReq.New = updatedApp

	// Return updated client configuration
	response := codersdk.OAuth2ClientConfiguration{
		ClientID:                updatedApp.ID.String(),
		ClientIDIssuedAt:        updatedApp.ClientIDIssuedAt.Time.Unix(),
		ClientSecretExpiresAt:   0, // No expiration for now
		RedirectURIs:            updatedApp.RedirectUris,
		ClientName:              updatedApp.Name,
		ClientURI:               updatedApp.ClientUri.String,
		LogoURI:                 updatedApp.LogoUri.String,
		TOSURI:                  updatedApp.TosUri.String,
		PolicyURI:               updatedApp.PolicyUri.String,
		JWKSURI:                 updatedApp.JwksUri.String,
		JWKS:                    updatedApp.Jwks.RawMessage,
		SoftwareID:              updatedApp.SoftwareID.String,
		SoftwareVersion:         updatedApp.SoftwareVersion.String,
		GrantTypes:              updatedApp.GrantTypes,
		ResponseTypes:           updatedApp.ResponseTypes,
		TokenEndpointAuthMethod: updatedApp.TokenEndpointAuthMethod.String,
		Scope:                   updatedApp.Scope.String,
		Contacts:                updatedApp.Contacts,
		RegistrationAccessToken: updatedApp.RegistrationAccessToken.String,
		RegistrationClientURI:   updatedApp.RegistrationClientUri.String,
	}

	httpapi.Write(ctx, rw, http.StatusOK, response)
}

// @Summary Delete OAuth2 client registration (RFC 7592)
// @ID delete-oauth2-client-configuration
// @Tags Enterprise
// @Param client_id path string true "Client ID"
// @Success 204
// @Router /oauth2/clients/{client_id} [delete]
func (api *API) deleteOAuth2ClientConfiguration(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	auditor := *api.Auditor.Load()
	aReq, commitAudit := audit.InitRequest[database.OAuth2ProviderApp](rw, &audit.RequestParams{
		Audit:   auditor,
		Log:     api.Logger,
		Request: r,
		Action:  database.AuditActionDelete,
	})
	defer commitAudit()

	// Extract client ID from URL path
	clientIDStr := chi.URLParam(r, "client_id")
	clientID, err := uuid.Parse(clientIDStr)
	if err != nil {
		writeOAuth2RegistrationError(ctx, rw, http.StatusBadRequest,
			"invalid_client_metadata", "Invalid client ID format")
		return
	}

	// Get existing app to verify it exists and is dynamically registered
	//nolint:gocritic // RFC 7592 endpoints need system access to retrieve dynamically registered clients
	existingApp, err := api.Database.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)
	if err == nil {
		aReq.Old = existingApp
	}
	if err != nil {
		if xerrors.Is(err, sql.ErrNoRows) {
			writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
				"invalid_token", "Client not found")
		} else {
			writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
				"server_error", "Failed to retrieve client")
		}
		return
	}

	// Check if client was dynamically registered
	if !existingApp.DynamicallyRegistered.Bool {
		writeOAuth2RegistrationError(ctx, rw, http.StatusForbidden,
			"invalid_token", "Client was not dynamically registered")
		return
	}

	// Delete the client and all associated data (tokens, secrets, etc.)
	//nolint:gocritic // RFC 7592 endpoints need system access to delete dynamically registered clients
	err = api.Database.DeleteOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)
	if err != nil {
		writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
			"server_error", "Failed to delete client")
		return
	}

	// Note: audit data already set above with aReq.Old = existingApp

	// Return 204 No Content as per RFC 7592
	rw.WriteHeader(http.StatusNoContent)
}

// requireRegistrationAccessToken middleware validates the registration access token for RFC 7592 endpoints
func (api *API) requireRegistrationAccessToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract client ID from URL path
		clientIDStr := chi.URLParam(r, "client_id")
		clientID, err := uuid.Parse(clientIDStr)
		if err != nil {
			writeOAuth2RegistrationError(ctx, rw, http.StatusBadRequest,
				"invalid_client_id", "Invalid client ID format")
			return
		}

		// Extract registration access token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
				"invalid_token", "Missing Authorization header")
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
				"invalid_token", "Authorization header must use Bearer scheme")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
				"invalid_token", "Missing registration access token")
			return
		}

		// Get the client and verify the registration access token
		//nolint:gocritic // RFC 7592 endpoints need system access to validate dynamically registered clients
		app, err := api.Database.GetOAuth2ProviderAppByClientID(dbauthz.AsSystemRestricted(ctx), clientID)
		if err != nil {
			if xerrors.Is(err, sql.ErrNoRows) {
				// Return 401 for authentication-related issues, not 404
				writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
					"invalid_token", "Client not found")
			} else {
				writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
					"server_error", "Failed to retrieve client")
			}
			return
		}

		// Check if client was dynamically registered
		if !app.DynamicallyRegistered.Bool {
			writeOAuth2RegistrationError(ctx, rw, http.StatusForbidden,
				"invalid_token", "Client was not dynamically registered")
			return
		}

		// Verify the registration access token
		if !app.RegistrationAccessToken.Valid {
			writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
				"server_error", "Client has no registration access token")
			return
		}

		// Compare the provided token with the stored hash
		valid, err := userpassword.Compare(app.RegistrationAccessToken.String, token)
		if err != nil {
			writeOAuth2RegistrationError(ctx, rw, http.StatusInternalServerError,
				"server_error", "Failed to verify registration access token")
			return
		}
		if !valid {
			writeOAuth2RegistrationError(ctx, rw, http.StatusUnauthorized,
				"invalid_token", "Invalid registration access token")
			return
		}

		// Token is valid, continue to the next handler
		next.ServeHTTP(rw, r)
	})
}
