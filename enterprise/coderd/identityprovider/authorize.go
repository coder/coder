package identityprovider

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)

type authorizeParams struct {
	clientID     string
	redirectURL  *url.URL
	responseType codersdk.OAuth2ProviderResponseType
	scope        []string
	state        string
}

func extractAuthorizeParams(r *http.Request, callbackURL string) (authorizeParams, []codersdk.ValidationError, error) {
	p := httpapi.NewQueryParamParser()
	vals := r.URL.Query()

	p.Required("state", "response_type", "client_id")

	// TODO: Can we make this a URL straight out of the database?
	cb, err := url.Parse(callbackURL)
	if err != nil {
		return authorizeParams{}, nil, err
	}
	params := authorizeParams{
		clientID:     p.String(vals, "", "client_id"),
		redirectURL:  p.URL(vals, cb, "redirect_uri"),
		responseType: httpapi.ParseCustom(p, vals, "", "response_type", httpapi.ParseEnum[codersdk.OAuth2ProviderResponseType]),
		scope:        p.Strings(vals, []string{}, "scope"),
		state:        p.String(vals, "", "state"),
	}

	// We add "redirected" when coming from the authorize page.
	_ = p.String(vals, "", "redirected")

	if err := validateRedirectURL(cb, params.redirectURL.String()); err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  "redirect_uri",
			Detail: fmt.Sprintf("Query param %q is invalid", "redirect_uri"),
		})
	}

	p.ErrorExcessParams(vals)
	return params, p.Errors, nil
}

/**
 * Authorize displays an HTML page for authorizing an application when the user
 * has first been redirected to this path and generates a code and redirects to
 * the app's callback URL after the user clicks "allow" on that page, which is
 * detected via the origin and referer headers.
 */
func Authorize(db database.Store, accessURL *url.URL) http.HandlerFunc {
	handler := func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		apiKey := httpmw.APIKey(r)
		app := httpmw.OAuth2ProviderApp(r)

		params, validationErrs, err := extractAuthorizeParams(r, app.CallbackURL)
		if err != nil {
			httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to validate query parameters.",
				Detail:  err.Error(),
			})
			return
		}
		if len(validationErrs) > 0 {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message:     "Invalid query params.",
				Validations: validationErrs,
			})
			return
		}

		// TODO: Ignoring scope for now, but should look into implementing.
		// 40 characters matches the length of GitHub's client secrets.
		rawCode, err := cryptorand.String(40)
		if err != nil {
			httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to generate OAuth2 app authorization code.",
			})
			return
		}
		hashedCode := Hash(rawCode, app.ID)
		err = db.InTx(func(tx database.Store) error {
			// Delete any previous codes.
			err = tx.DeleteOAuth2ProviderAppCodesByAppAndUserID(ctx, database.DeleteOAuth2ProviderAppCodesByAppAndUserIDParams{
				AppID:  app.ID,
				UserID: apiKey.UserID,
			})
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return xerrors.Errorf("delete oauth2 app codes: %w", err)
			}

			// Insert the new code.
			_, err = tx.InsertOAuth2ProviderAppCode(ctx, database.InsertOAuth2ProviderAppCodeParams{
				ID:        uuid.New(),
				CreatedAt: dbtime.Now(),
				// TODO: Configurable expiration?  Ten minutes matches GitHub.
				ExpiresAt:    dbtime.Now().Add(time.Duration(10) * time.Minute),
				HashedSecret: hashedCode[:],
				AppID:        app.ID,
				UserID:       apiKey.UserID,
			})
			if err != nil {
				return xerrors.Errorf("insert oauth2 authorization code: %w", err)
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
		newQuery.Add("code", rawCode)
		newQuery.Add("state", params.state)
		params.redirectURL.RawQuery = newQuery.Encode()

		http.Redirect(rw, r, params.redirectURL.String(), http.StatusTemporaryRedirect)
	}

	// Always wrap with its custom mw.
	return authorizeMW(accessURL)(http.HandlerFunc(handler)).ServeHTTP
}

// validateRedirectURL validates that the redirectURL is contained in baseURL.
func validateRedirectURL(baseURL *url.URL, redirectURL string) error {
	redirect, err := url.Parse(redirectURL)
	if err != nil {
		return err
	}
	// It can be a sub-directory but not a sub-domain, as we have apps on
	// sub-domains so it seems too dangerous.
	if redirect.Host != baseURL.Host || !strings.HasPrefix(redirect.Path, baseURL.Path) {
		return xerrors.New("invalid redirect URL")
	}
	return nil
}
