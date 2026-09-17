package oauth2provider

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/db2sdk"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
)

// resolveRedirectURIs returns the redirect URIs an app should have after a
// create or update request. The first entry is the primary.
//
// If the request has redirectURIs, that list is used. If it also has
// callbackURL, callbackURL is moved to the front of the list.
// If the request has only callbackURL, an update replaces the first stored
// URI with callbackURL and keeps the rest. A create uses callbackURL alone.
// If the request has neither, the stored list is kept.
// stored is nil on a create.
//
// Only the list-only and callback-only shapes have callers today. The admin
// API gains redirect_uris in the next PR and sends the other two.
func resolveRedirectURIs(callbackURL string, redirectURIs, stored []string) []string {
	list := slice.Unique(redirectURIs)
	if len(list) == 0 && len(stored) > 0 {
		if callbackURL == "" {
			return stored
		}
		list = stored[1:]
	}
	if callbackURL != "" {
		list = slices.DeleteFunc(slices.Clone(list), func(s string) bool { return s == callbackURL })
		list = append([]string{callbackURL}, list...)
	}
	return list
}

// validateRedirectURIs checks each URI the same way callback_url is checked:
// it must parse, use an allowed scheme, and name a target. For a public app,
// the redirect URI rules from dynamic client registration also apply.
//
// URIs already in stored are skipped, and the count cap applies only when the
// list grew. This keeps apps registered before the caps existed editable.
func validateRedirectURIs(uris []string, clientType codersdk.OAuth2ClientType, fromCallback string, stored []string) []codersdk.ValidationError {
	// A failure on the request's callback_url is reported against that field
	// and without a list index, since the caller never sent a list.
	invalid := func(i int, uri, detail string) []codersdk.ValidationError {
		if uri != "" && uri == fromCallback {
			return []codersdk.ValidationError{{Field: "callback_url", Detail: "callback URL " + detail}}
		}
		return []codersdk.ValidationError{{
			Field:  "redirect_uris",
			Detail: fmt.Sprintf("redirect URI at index %d %s", i, detail),
		}}
	}
	if len(uris) == 0 {
		return []codersdk.ValidationError{{
			Field:  "redirect_uris",
			Detail: "at least one redirect URI is required",
		}}
	}
	if len(uris) > codersdk.OAuth2RedirectURIsMaxCount && len(uris) > len(stored) {
		return []codersdk.ValidationError{{
			Field:  "redirect_uris",
			Detail: fmt.Sprintf("at most %d redirect URIs are allowed", codersdk.OAuth2RedirectURIsMaxCount),
		}}
	}
	for i, uri := range uris {
		if slices.Contains(stored, uri) {
			continue
		}
		if len(uri) > codersdk.OAuth2RedirectURIMaxBytes {
			return invalid(i, uri, fmt.Sprintf("must be at most %d bytes", codersdk.OAuth2RedirectURIMaxBytes))
		}
		if err := codersdk.ValidateOAuth2CallbackURL(uri); err != nil {
			return invalid(i, uri, err.Error())
		}
		if clientType != codersdk.OAuth2ClientTypePublic {
			continue
		}
		if err := codersdk.ValidateRedirectURI(uri, clientType); err != nil {
			return invalid(i, uri, "is rejected for a public client: "+err.Error())
		}
	}
	return nil
}

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

		sdkApps := make([]codersdk.OAuth2ProviderApp, 0, len(userApps))
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
		redirectURIs := resolveRedirectURIs(req.CallbackURL, nil, nil)
		if errs := validateRedirectURIs(redirectURIs, codersdk.OAuth2ClientTypeConfidential, req.CallbackURL, nil); errs != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Validation failed.",
				Validations: errs,
			})
			return
		}
		app, err := db.InsertOAuth2ProviderApp(ctx, database.InsertOAuth2ProviderAppParams{
			ID:                      uuid.New(),
			CreatedAt:               dbtime.Now(),
			UpdatedAt:               dbtime.Now(),
			Name:                    req.Name,
			Icon:                    req.Icon,
			CallbackURL:             redirectURIs[0],
			RedirectUris:            redirectURIs,
			ClientType:              database.OAuth2ProviderAppClientTypeConfidential,
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
		clientType := codersdk.OAuth2ClientTypeConfidential
		if app.IsPublic() {
			clientType = codersdk.OAuth2ClientTypePublic
		}
		stored := app.RegisteredRedirectURIs()
		redirectURIs := resolveRedirectURIs(req.CallbackURL, nil, stored)
		if errs := validateRedirectURIs(redirectURIs, clientType, req.CallbackURL, stored); errs != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Validation failed.",
				Validations: errs,
			})
			return
		}
		app, err := db.UpdateOAuth2ProviderAppByID(ctx, database.UpdateOAuth2ProviderAppByIDParams{
			ID:                      app.ID,
			UpdatedAt:               dbtime.Now(),
			Name:                    req.Name,
			Icon:                    req.Icon,
			CallbackURL:             redirectURIs[0],
			RedirectUris:            redirectURIs,
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
