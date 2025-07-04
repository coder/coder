package oauth2provider

import (
	"database/sql"
	"errors"
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
)

type authorizeParams struct {
	clientID            string
	redirectURL         *url.URL
	responseType        codersdk.OAuth2ProviderResponseType
	scope               []string
	state               string
	resource            string // RFC 8707 resource indicator
	codeChallenge       string // PKCE code challenge
	codeChallengeMethod string // PKCE challenge method
}

func extractAuthorizeParams(r *http.Request, callbackURL *url.URL) (authorizeParams, []codersdk.ValidationError, error) {
	p := httpapi.NewQueryParamParser()
	vals := r.URL.Query()

	p.RequiredNotEmpty("state", "response_type", "client_id")

	params := authorizeParams{
		clientID:            p.String(vals, "", "client_id"),
		redirectURL:         p.RedirectURL(vals, callbackURL, "redirect_uri"),
		responseType:        httpapi.ParseCustom(p, vals, "", "response_type", httpapi.ParseEnum[codersdk.OAuth2ProviderResponseType]),
		scope:               p.Strings(vals, []string{}, "scope"),
		state:               p.String(vals, "", "state"),
		resource:            p.String(vals, "", "resource"),
		codeChallenge:       p.String(vals, "", "code_challenge"),
		codeChallengeMethod: p.String(vals, "", "code_challenge_method"),
	}
	// Validate resource indicator syntax (RFC 8707): must be absolute URI without fragment
	if err := validateResourceParameter(params.resource); err != nil {
		p.Errors = append(p.Errors, codersdk.ValidationError{
			Field:  "resource",
			Detail: "must be an absolute URI without fragment",
		})
	}

	p.ErrorExcessParams(vals)
	if len(p.Errors) > 0 {
		// Create a readable error message with validation details
		var errorDetails []string
		for _, err := range p.Errors {
			errorDetails = append(errorDetails, err.Error())
		}
		errorMsg := "Invalid query params: " + strings.Join(errorDetails, ", ")
		return authorizeParams{}, p.Errors, xerrors.Errorf(errorMsg)
	}
	return params, nil, nil
}

// ShowAuthorizePage handles GET /oauth2/authorize requests to display the HTML authorization page.
// It uses authorizeMW which intercepts GET requests to show the authorization form.
func ShowAuthorizePage(db database.Store, accessURL *url.URL) http.HandlerFunc {
	handler := authorizeMW(accessURL)(ProcessAuthorize(db, accessURL))
	return handler.ServeHTTP
}

// ProcessAuthorize handles POST /oauth2/authorize requests to process the user's authorization decision
// and generate an authorization code. GET requests are handled by authorizeMW.
func ProcessAuthorize(db database.Store, accessURL *url.URL) http.HandlerFunc {
	handler := func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		apiKey := httpmw.APIKey(r)
		app := httpmw.OAuth2ProviderApp(r)

		callbackURL, err := url.Parse(app.CallbackURL)
		if err != nil {
			httpapi.WriteOAuth2Error(r.Context(), rw, http.StatusInternalServerError, "server_error", "Failed to validate query parameters")
			return
		}

		params, _, err := extractAuthorizeParams(r, callbackURL)
		if err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		// Validate PKCE for public clients (MCP requirement)
		if params.codeChallenge != "" {
			// If code_challenge is provided but method is not, default to S256
			if params.codeChallengeMethod == "" {
				params.codeChallengeMethod = "S256"
			}
			if params.codeChallengeMethod != "S256" {
				httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, "invalid_request", "Invalid code_challenge_method: only S256 is supported")
				return
			}
		}

		// TODO: Ignoring scope for now, but should look into implementing.
		code, err := GenerateSecret()
		if err != nil {
			httpapi.WriteOAuth2Error(r.Context(), rw, http.StatusInternalServerError, "server_error", "Failed to generate OAuth2 app authorization code")
			return
		}
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
				// This timeout is only for the code that will be exchanged for the
				// access token, not the access token itself.  It does not need to be
				// long-lived because normally it will be exchanged immediately after it
				// is received.  If the application does wait before exchanging the
				// token (for example suppose they ask the user to confirm and the user
				// has left) then they can just retry immediately and get a new code.
				ExpiresAt:           dbtime.Now().Add(time.Duration(10) * time.Minute),
				SecretPrefix:        []byte(code.Prefix),
				HashedSecret:        []byte(code.Hashed),
				AppID:               app.ID,
				UserID:              apiKey.UserID,
				ResourceUri:         sql.NullString{String: params.resource, Valid: params.resource != ""},
				CodeChallenge:       sql.NullString{String: params.codeChallenge, Valid: params.codeChallenge != ""},
				CodeChallengeMethod: sql.NullString{String: params.codeChallengeMethod, Valid: params.codeChallengeMethod != ""},
			})
			if err != nil {
				return xerrors.Errorf("insert oauth2 authorization code: %w", err)
			}

			return nil
		}, nil)
		if err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError, "server_error", "Failed to generate OAuth2 authorization code")
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
