package oauth2provider

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)

// ListApps returns an http.HandlerFunc that handles GET /oauth2-provider/apps
func ListApps(db database.Store, accessURL *url.URL) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		rawUserID := r.URL.Query().Get("user_id")
		if rawUserID == "" {
			dbApps, err := db.GetOAuth2ProviderApps(ctx)
			if err != nil {
				httpapi.InternalServerError(rw, err)
				return
			}
			httpapi.Write(ctx, rw, http.StatusOK, db2sdk.OAuth2ProviderApps(accessURL, dbApps))
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

		userApps, err := db.GetOAuth2ProviderAppsByUserID(ctx, userID)
		if err != nil {
			httpapi.InternalServerError(rw, err)
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
		app := httpmw.OAuth2ProviderApp(r)
		httpapi.Write(ctx, rw, http.StatusOK, db2sdk.OAuth2ProviderApp(accessURL, app))
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
		app, err := db.InsertOAuth2ProviderApp(ctx, database.InsertOAuth2ProviderAppParams{
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
			RegistrationAccessToken: nil,
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
		app, err := db.UpdateOAuth2ProviderAppByID(ctx, database.UpdateOAuth2ProviderAppByIDParams{
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
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error deleting OAuth2 application.",
				Detail:  err.Error(),
			})
			return
		}
		rw.WriteHeader(http.StatusNoContent)
	}
}
