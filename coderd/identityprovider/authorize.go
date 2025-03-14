package identityprovider

import (
	"fmt"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
)
type authorizeParams struct {
	clientID     string
	redirectURL  *url.URL

	responseType codersdk.OAuth2ProviderResponseType
	scope        []string
	state        string
}
func extractAuthorizeParams(r *http.Request, callbackURL *url.URL) (authorizeParams, []codersdk.ValidationError, error) {
	p := httpapi.NewQueryParamParser()
	vals := r.URL.Query()
	p.RequiredNotEmpty("state", "response_type", "client_id")

	params := authorizeParams{
		clientID:     p.String(vals, "", "client_id"),
		redirectURL:  p.RedirectURL(vals, callbackURL, "redirect_uri"),
		responseType: httpapi.ParseCustom(p, vals, "", "response_type", httpapi.ParseEnum[codersdk.OAuth2ProviderResponseType]),

		scope:        p.Strings(vals, []string{}, "scope"),
		state:        p.String(vals, "", "state"),

	}
	// We add "redirected" when coming from the authorize page.
	_ = p.String(vals, "", "redirected")
	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		return authorizeParams{}, p.Errors, fmt.Errorf("invalid query params: %w", p.Errors)
	}
	return params, nil, nil

}
// Authorize displays an HTML page for authorizing an application when the user
// has first been redirected to this path and generates a code and redirects to

// the app's callback URL after the user clicks "allow" on that page, which is
// detected via the origin and referer headers.
func Authorize(db database.Store, accessURL *url.URL) http.HandlerFunc {
	handler := func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		apiKey := httpmw.APIKey(r)
		app := httpmw.OAuth2ProviderApp(r)

		callbackURL, err := url.Parse(app.CallbackURL)
		if err != nil {
			httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to validate query parameters.",
				Detail:  err.Error(),
			})
			return
		}
		params, validationErrs, err := extractAuthorizeParams(r, callbackURL)
		if err != nil {

			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Invalid query params.",
				Detail:      err.Error(),
				Validations: validationErrs,
			})
			return
		}
		// TODO: Ignoring scope for now, but should look into implementing.
		code, err := GenerateSecret()

		if err != nil {
			httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to generate OAuth2 app authorization code.",
			})
			return
		}
		err = db.InTx(func(tx database.Store) error {
			// Delete any previous codes.
			err = tx.DeleteOAuth2ProviderAppCodesByAppAndUserID(ctx, database.DeleteOAuth2ProviderAppCodesByAppAndUserIDParams{
				AppID:  app.ID,

				UserID: apiKey.UserID,
			})
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("delete oauth2 app codes: %w", err)
			}
			// Insert the new code.
			_, err = tx.InsertOAuth2ProviderAppCode(ctx, database.InsertOAuth2ProviderAppCodeParams{
				ID:        uuid.New(),
				CreatedAt: dbtime.Now(),
				// TODO: Configurable expiration?  Ten minutes matches GitHub.
				// This timeout is only for the code that will be exchanged for the
				// access token, not the access token itself.  It does not need to be
				// long-lived because normally it will be exchanged immediately after it
				// is received.  If the application does wait before exchanging the
				// token (for example suppose they ask the user to confirm and the user
				// has left) then they can just retry immediately and get a new code.
				ExpiresAt:    dbtime.Now().Add(time.Duration(10) * time.Minute),
				SecretPrefix: []byte(code.Prefix),

				HashedSecret: []byte(code.Hashed),
				AppID:        app.ID,
				UserID:       apiKey.UserID,
			})
			if err != nil {
				return fmt.Errorf("insert oauth2 authorization code: %w", err)
			}
			return nil
		}, nil)
		if err != nil {
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to generate OAuth2 authorization code.",
				Detail:  err.Error(),
			})
			return
		}
		newQuery := params.redirectURL.Query()
		newQuery.Add("code", code.Formatted)
		newQuery.Add("state", params.state)
		params.redirectURL.RawQuery = newQuery.Encode()
		http.Redirect(rw, r, params.redirectURL.String(), http.StatusTemporaryRedirect)

	}
	// Always wrap with its custom mw.
	return authorizeMW(accessURL)(http.HandlerFunc(handler)).ServeHTTP
}
