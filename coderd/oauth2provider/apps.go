package oauth2provider

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
)

// parseScopeString parses a space-delimited OAuth2 scope string into APIKeyScope array
func parseScopeString(scope string) []database.APIKeyScope {
	if scope == "" {
		return []database.APIKeyScope{}
	}

	scopeTokens := strings.Split(strings.TrimSpace(scope), " ")
	scopes := make([]database.APIKeyScope, 0, len(scopeTokens))

	for _, token := range scopeTokens {
		token = strings.TrimSpace(token)
		if token != "" {
			// Convert to database APIKeyScope, only include valid scopes
			dbScope := database.APIKeyScope(token)
			if dbScope.Valid() {
				scopes = append(scopes, dbScope)
			}
		}
	}

	return scopes
}

// ListApps returns an http.HandlerFunc that handles GET /oauth2-provider/apps
func ListApps(db database.Store, accessURL *url.URL, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		rawUserID := r.URL.Query().Get("user_id")
		rawOwnerID := r.URL.Query().Get("owner_id")

		// If neither filter is provided, return all apps
		if rawUserID == "" && rawOwnerID == "" {
			dbApps, err := db.GetOAuth2ProviderApps(ctx)
			if err != nil {
				logger.Error(ctx, "failed to get OAuth2 provider apps", slog.Error(err))
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error retrieving OAuth2 applications.",
				})
				return
			}
			httpapi.Write(ctx, rw, http.StatusOK, db2sdk.OAuth2ProviderAppsRows(accessURL, dbApps))
			return
		}

		// Handle owner_id filter - apps created by the user
		if rawOwnerID != "" {
			ownerID, err := uuid.Parse(rawOwnerID)
			if err != nil {
				logger.Warn(ctx, "invalid owner UUID provided", slog.F("owner_id", rawOwnerID), slog.Error(err))
				httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
					Message: "Invalid owner UUID",
					Detail:  fmt.Sprintf("queried owner_id=%q", rawOwnerID),
				})
				return
			}

			ownedApps, err := db.GetOAuth2ProviderAppsByOwnerID(ctx, uuid.NullUUID{
				UUID:  ownerID,
				Valid: true,
			})
			if err != nil {
				logger.Error(ctx, "failed to get OAuth2 provider apps by owner", slog.F("owner_id", ownerID), slog.Error(err))
				httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
					Message: "Internal error retrieving OAuth2 applications.",
				})
				return
			}

			httpapi.Write(ctx, rw, http.StatusOK, db2sdk.OAuth2ProviderAppsByOwnerIDRows(accessURL, ownedApps))
			return
		}

		// Handle user_id filter - apps the user has authorized (has tokens for)
		userID, err := uuid.Parse(rawUserID)
		if err != nil {
			logger.Warn(ctx, "invalid user UUID provided", slog.F("user_id", rawUserID), slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid user UUID",
				Detail:  fmt.Sprintf("queried user_id=%q", rawUserID),
			})
			return
		}

		userApps, err := db.GetOAuth2ProviderAppsByUserID(ctx, userID)
		if err != nil {
			logger.Error(ctx, "failed to get OAuth2 provider apps by user", slog.F("user_id", userID), slog.Error(err))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error retrieving OAuth2 applications.",
			})
			return
		}

		var sdkApps []codersdk.OAuth2ProviderApp
		for _, app := range userApps {
			sdkApps = append(sdkApps, db2sdk.OAuth2ProviderApp(accessURL, app.OAuth2ProviderApp))
		}
		httpapi.Write(ctx, rw, http.StatusOK, sdkApps)
	}
}

// GetApp returns an http.HandlerFunc that handles GET /oauth2-provider/apps/{app}
func GetApp(accessURL *url.URL) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		appRow := httpmw.OAuth2ProviderAppRow(r)
		httpapi.Write(ctx, rw, http.StatusOK, db2sdk.OAuth2ProviderAppRow(accessURL, appRow))
	}
}

// CreateApp returns an http.HandlerFunc that handles POST /oauth2-provider/apps
func CreateApp(db database.Store, accessURL *url.URL, auditor *audit.Auditor, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var (
			ctx               = r.Context()
			aReq, commitAudit = audit.InitRequest[database.OAuth2ProviderApp](rw, &audit.RequestParams{
				Audit:   *auditor,
				Log:     logger,
				Request: r,
				Action:  database.AuditActionCreate,
			})
		)
		defer commitAudit()
		var req codersdk.PostOAuth2ProviderAppRequest
		if !httpapi.Read(ctx, rw, r, &req) {
			return
		}

		// Validate grant types and redirect URI requirements
		if err := req.Validate(); err != nil {
			logger.Warn(ctx, "invalid OAuth2 application request", slog.Error(err), slog.F("request", req))
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid OAuth2 application request.",
				Detail:  err.Error(),
			})
			return
		}

		// Determine grant types and user ownership
		grantTypes := req.GrantTypes
		var userID uuid.NullUUID

		switch {
		case len(grantTypes) == 0:
			// Default behavior: authorization_code + refresh_token (system-scoped)
			grantTypes = []codersdk.OAuth2ProviderGrantType{codersdk.OAuth2ProviderGrantTypeAuthorizationCode, codersdk.OAuth2ProviderGrantTypeRefreshToken}
			userID = uuid.NullUUID{Valid: false} // NULL - system level
		case slices.Contains(grantTypes, codersdk.OAuth2ProviderGrantTypeClientCredentials):
			// Client credentials apps belong to creating user
			apiKey := httpmw.APIKey(r)
			userID = uuid.NullUUID{UUID: apiKey.UserID, Valid: true}
		default:
			// Authorization/device flows are system-level
			userID = uuid.NullUUID{Valid: false} // NULL
		}

		app, err := db.InsertOAuth2ProviderApp(ctx, database.InsertOAuth2ProviderAppParams{
			ID:                      uuid.New(),
			CreatedAt:               dbtime.Now(),
			UpdatedAt:               dbtime.Now(),
			Name:                    req.Name,
			Icon:                    req.Icon,
			RedirectUris:            req.RedirectURIs,
			ClientType:              sql.NullString{String: "confidential", Valid: true},
			DynamicallyRegistered:   sql.NullBool{Bool: false, Valid: true},
			UserID:                  userID,
			ClientIDIssuedAt:        sql.NullTime{},
			ClientSecretExpiresAt:   sql.NullTime{},
			GrantTypes:              codersdk.OAuth2ProviderGrantTypesToStrings(grantTypes),
			ResponseTypes:           []string{string(codersdk.OAuth2ProviderResponseTypeCode)},
			TokenEndpointAuthMethod: sql.NullString{String: "client_secret_post", Valid: true},
			Scopes:                  []database.APIKeyScope{}, // New scopes array (empty for now, OAuth2 apps don't specify scopes at creation)
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
			if rbac.IsUnauthorizedError(err) {
				logger.Debug(ctx, "unauthorized to create OAuth2 application", slog.Error(err), slog.F("app_name", req.Name))
				httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
					Message: "You are not authorized to create this type of OAuth2 application.",
				})
				return
			}
			logger.Error(ctx, "failed to create OAuth2 application", slog.Error(err), slog.F("app_name", req.Name))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error creating OAuth2 application.",
			})
			return
		}
		aReq.New = app
		httpapi.Write(ctx, rw, http.StatusCreated, db2sdk.OAuth2ProviderApp(accessURL, app))
	}
}

// UpdateApp returns an http.HandlerFunc that handles PUT /oauth2-provider/apps/{app}
func UpdateApp(db database.Store, accessURL *url.URL, auditor *audit.Auditor, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var (
			ctx               = r.Context()
			app               = httpmw.OAuth2ProviderApp(r)
			aReq, commitAudit = audit.InitRequest[database.OAuth2ProviderApp](rw, &audit.RequestParams{
				Audit:   *auditor,
				Log:     logger,
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

		// Validate the update request
		if err := req.Validate(); err != nil {
			logger.Warn(ctx, "invalid OAuth2 application update request", slog.Error(err), slog.F("app_id", app.ID), slog.F("request", req))
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid OAuth2 application update request.",
				Detail:  err.Error(),
			})
			return
		}

		// Determine grant types to use (allow updates if provided)
		grantTypes := app.GrantTypes // Default to existing (strings from database)
		if len(req.GrantTypes) > 0 {
			grantTypes = codersdk.OAuth2ProviderGrantTypesToStrings(req.GrantTypes)
		}

		app, err := db.UpdateOAuth2ProviderAppByID(ctx, database.UpdateOAuth2ProviderAppByIDParams{
			ID:                      app.ID,
			UpdatedAt:               dbtime.Now(),
			Name:                    req.Name,
			Icon:                    req.Icon,
			RedirectUris:            req.RedirectURIs,
			ClientType:              app.ClientType,              // Keep existing value
			DynamicallyRegistered:   app.DynamicallyRegistered,   // Keep existing value
			ClientSecretExpiresAt:   app.ClientSecretExpiresAt,   // Keep existing value
			GrantTypes:              grantTypes,                  // Allow updates
			ResponseTypes:           app.ResponseTypes,           // Keep existing value
			TokenEndpointAuthMethod: app.TokenEndpointAuthMethod, // Keep existing value
			Scopes:                  app.Scopes,                  // Keep existing value
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
			logger.Error(ctx, "failed to update OAuth2 application", slog.Error(err), slog.F("app_id", app.ID), slog.F("app_name", req.Name))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error updating OAuth2 application.",
			})
			return
		}
		aReq.New = app
		httpapi.Write(ctx, rw, http.StatusOK, db2sdk.OAuth2ProviderApp(accessURL, app))
	}
}

// DeleteApp returns an http.HandlerFunc that handles DELETE /oauth2-provider/apps/{app}
func DeleteApp(db database.Store, auditor *audit.Auditor, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		var (
			ctx               = r.Context()
			app               = httpmw.OAuth2ProviderApp(r)
			aReq, commitAudit = audit.InitRequest[database.OAuth2ProviderApp](rw, &audit.RequestParams{
				Audit:   *auditor,
				Log:     logger,
				Request: r,
				Action:  database.AuditActionDelete,
			})
		)
		aReq.Old = app
		defer commitAudit()
		err := db.DeleteOAuth2ProviderAppByID(ctx, app.ID)
		if err != nil {
			logger.Error(ctx, "failed to delete OAuth2 application", slog.Error(err), slog.F("app_id", app.ID), slog.F("app_name", app.Name))
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error deleting OAuth2 application.",
			})
			return
		}
		rw.WriteHeader(http.StatusNoContent)
	}
}
