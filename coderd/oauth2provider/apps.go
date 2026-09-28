package oauth2provider

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

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

// errRedirectURIValidation ends the update transaction when the resolved list
// fails validation. The handler reports the collected validation errors instead.
var errRedirectURIValidation = xerrors.New("redirect URI validation failed")

// validateRedirectURIFieldsAgree returns an error when a request sets both
// callback_url and redirect_uris but they name a different first URI. The two
// fields mean the same thing, so a mismatch means the client edited one and
// forgot the other. Rejecting it avoids guessing which edit was intended.
func validateRedirectURIFieldsAgree(callbackURL string, redirectURIs []string) []codersdk.ValidationError {
	if callbackURL == "" || len(redirectURIs) == 0 || redirectURIs[0] == callbackURL {
		return nil
	}
	return []codersdk.ValidationError{{
		Field:  "callback_url",
		Detail: "callback URL must equal the first redirect URI when both are sent",
	}}
}

// resolveRedirectURIs returns the redirect URIs an app should have after a
// create or update request. The first entry is the primary.
//
// Sending redirectURIs replaces the whole stored list. Sending only
// callbackURL replaces the first stored URI and keeps the rest. Sending
// neither keeps the stored list. stored is nil on a create.
func resolveRedirectURIs(callbackURL string, redirectURIs, stored []string) []string {
	if redirectURIs != nil {
		return slice.Unique(redirectURIs)
	}
	if callbackURL == "" {
		return stored
	}
	if len(stored) == 0 {
		return []string{callbackURL}
	}
	rest := slices.DeleteFunc(slices.Clone(stored[1:]), func(s string) bool { return s == callbackURL })
	return append([]string{callbackURL}, rest...)
}

// emptyRedirectURIsDetail returns the error detail for an empty resolved list.
func emptyRedirectURIsDetail(fromCallback string) string {
	if fromCallback == "" {
		return "at least one redirect URI is required"
	}
	return "redirect_uris was sent as an empty list, which overrides callback_url; send at least one redirect URI, or omit redirect_uris to use callback_url"
}

// validateAppRedirectURIFields checks the list an admin request resolved to
// and reports each failure against the request field that caused it. Every
// entry passes both ValidateRedirectURIShape and ValidateRedirectURI, so an
// admin cannot store a redirect URI that dynamic client registration would
// refuse for the same client type.
//
// Stored URIs are checked again on every update, so an app that predates the
// caps, or that stored a cleartext http URI while this path did not check
// the scheme, cannot be saved until a request sends a list that passes.
func validateAppRedirectURIFields(uris []string, clientType codersdk.OAuth2ClientType, fromCallback string) []codersdk.ValidationError {
	// A failure on the request's callback_url is reported against that field
	// and without a row number, since the caller never sent a list. Rows are
	// numbered from one to match the labels the web UI shows.
	invalid := func(i int, uri, detail string) []codersdk.ValidationError {
		if uri != "" && uri == fromCallback {
			return []codersdk.ValidationError{{Field: "callback_url", Detail: "callback URL " + detail}}
		}
		return []codersdk.ValidationError{{
			Field:  "redirect_uris",
			Detail: fmt.Sprintf("redirect URI %d %s", i+1, detail),
		}}
	}
	if len(uris) == 0 {
		return []codersdk.ValidationError{{
			Field:  "redirect_uris",
			Detail: emptyRedirectURIsDetail(fromCallback),
		}}
	}
	if len(uris) > codersdk.OAuth2RedirectURIsMaxCount {
		return []codersdk.ValidationError{{
			Field:  "redirect_uris",
			Detail: fmt.Sprintf("at most %d redirect URIs are allowed", codersdk.OAuth2RedirectURIsMaxCount),
		}}
	}
	for i, uri := range uris {
		if len(uri) > codersdk.OAuth2RedirectURIMaxBytes {
			return invalid(i, uri, fmt.Sprintf("must be at most %d bytes", codersdk.OAuth2RedirectURIMaxBytes))
		}
		if err := codersdk.ValidateRedirectURIShape(uri); err != nil {
			return invalid(i, uri, err.Error())
		}
		// The shape check runs first because it names the more specific
		// reason for a malformed URI.
		if err := codersdk.ValidateRedirectURI(uri, clientType); err != nil {
			return invalid(i, uri, err.Error())
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

// scopeAllowlist wraps a scope list for storage. Every write path stores the
// spelling as given; readers canonicalize. An empty list stores as an empty,
// valid string, meaning no allowlist.
func scopeAllowlist(raw string) sql.NullString {
	return sql.NullString{
		String: raw,
		Valid:  true,
	}
}

// writeInvalidScopeError writes a 400 when a scope list is too large and
// reports whether it did. The check is explicit rather than a validate tag so
// the response names the limit instead of echoing the value back.
func writeInvalidScopeError(ctx context.Context, rw http.ResponseWriter, raw string) bool {
	err := codersdk.ValidateOAuth2ScopeList(raw)
	if err == nil {
		return false
	}
	httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
		Message: "Invalid scope.",
		Validations: []codersdk.ValidationError{{
			Field:  "scope",
			Detail: err.Error(),
		}},
	})
	return true
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
		redirectURIs := resolveRedirectURIs(req.CallbackURL, req.RedirectURIs, nil)
		errs := validateRedirectURIFieldsAgree(req.CallbackURL, req.RedirectURIs)
		if errs == nil {
			errs = validateAppRedirectURIFields(redirectURIs, codersdk.OAuth2ClientTypeConfidential, req.CallbackURL)
		}
		if errs != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Validation failed.",
				Validations: errs,
			})
			return
		}
		if writeInvalidScopeError(ctx, rw, req.Scope) {
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
			Scope:                   scopeAllowlist(req.Scope),
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
		if errs := validateRedirectURIFieldsAgree(req.CallbackURL, req.RedirectURIs); errs != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Validation failed.",
				Validations: errs,
			})
			return
		}
		if req.Scope != nil && writeInvalidScopeError(ctx, rw, *req.Scope) {
			return
		}

		var (
			updated        database.OAuth2ProviderApp
			validationErrs []codersdk.ValidationError
		)
		err := db.InTx(func(tx database.Store) error {
			// The middleware read is a snapshot. When the request omits the
			// URI fields, the stored list is written back, so it must come
			// from a locked read or a concurrent change to it would be undone.
			current, err := tx.GetOAuth2ProviderAppByIDForUpdate(ctx, app.ID)
			if err != nil {
				return xerrors.Errorf("get OAuth2 app for update: %w", err)
			}
			aReq.Old = current
			clientType := codersdk.OAuth2ClientTypeConfidential
			if current.IsPublic() {
				clientType = codersdk.OAuth2ClientTypePublic
			}
			redirectURIs := resolveRedirectURIs(req.CallbackURL, req.RedirectURIs, current.RegisteredRedirectURIs())
			validationErrs = validateAppRedirectURIFields(redirectURIs, clientType, req.CallbackURL)
			if validationErrs != nil {
				return errRedirectURIValidation
			}
			scope := current.Scope // Keep existing value
			if req.Scope != nil {
				scope = scopeAllowlist(*req.Scope)
			}
			updated, err = tx.UpdateOAuth2ProviderAppByID(ctx, database.UpdateOAuth2ProviderAppByIDParams{
				ID:                      current.ID,
				UpdatedAt:               dbtime.Now(),
				Name:                    req.Name,
				Icon:                    req.Icon,
				CallbackURL:             redirectURIs[0],
				RedirectUris:            redirectURIs,
				ClientType:              current.ClientType,              // Keep existing value
				DynamicallyRegistered:   current.DynamicallyRegistered,   // Keep existing value
				ClientSecretExpiresAt:   current.ClientSecretExpiresAt,   // Keep existing value
				GrantTypes:              current.GrantTypes,              // Keep existing value
				ResponseTypes:           current.ResponseTypes,           // Keep existing value
				TokenEndpointAuthMethod: current.TokenEndpointAuthMethod, // Keep existing value
				Scope:                   scope,
				Contacts:                current.Contacts,        // Keep existing value
				ClientUri:               current.ClientUri,       // Keep existing value
				LogoUri:                 current.LogoUri,         // Keep existing value
				TosUri:                  current.TosUri,          // Keep existing value
				PolicyUri:               current.PolicyUri,       // Keep existing value
				JwksUri:                 current.JwksUri,         // Keep existing value
				Jwks:                    current.Jwks,            // Keep existing value
				SoftwareID:              current.SoftwareID,      // Keep existing value
				SoftwareVersion:         current.SoftwareVersion, // Keep existing value
			})
			if err != nil {
				return xerrors.Errorf("update OAuth2 app: %w", err)
			}
			return nil
		}, nil)
		if validationErrs != nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Validation failed.",
				Validations: validationErrs,
			})
			return
		}
		if err != nil {
			if httpapi.Is404Error(err) {
				httpapi.ResourceNotFound(rw)
				return
			}
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Internal error updating OAuth2 application.",
				Detail:  err.Error(),
			})
			return
		}
		aReq.New = updated
		httpapi.Write(ctx, rw, http.StatusOK, db2sdk.OAuth2ProviderApp(accessURL, updated))
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
