package oauth2provider

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	htmltemplate "html/template"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/justinas/nosurf"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/site"
)

// Rejection reasons from negotiateScope.
var (
	// The name is not in the external scope catalog: unknown, or internal-only.
	errUnknownScope = xerrors.New("unknown or unsupported scope")
	// Every entry in the app's allowlist falls outside the catalog.
	errNoGrantableScope = xerrors.New("none of the scopes registered for this app are supported by this deployment; change the app's registered scopes to supported ones")
	// The scope expands to permissions the allowlist does not cover.
	errScopeNotAllowed = xerrors.New("scope requests permissions beyond this app's allowed scopes")
	// A comparison that failed outright. The underlying error names RBAC
	// internals, so it is logged rather than rendered.
	errCoverageUndecidable = xerrors.New("scope coverage against this app's allowed scopes could not be determined")
)

// canonicalScopes rewrites each name to its api_key_scope enum spelling and
// drops duplicates. The aliases `all` and `application_connect` pass validation
// but are not enum members, so they must be rewritten before being stored.
func canonicalScopes(names []string) []string {
	canonical := make([]string, 0, len(names))
	for _, name := range names {
		canonical = append(canonical, string(rbac.CanonicalScopeName(rbac.ScopeName(name))))
	}
	return slice.Unique(canonical)
}

// noScopeAllowlist reports whether an app has no scope allowlist. NULL and ""
// are the same state: admin-created apps store NULL, DCR-registered apps store
// a possibly empty req.Scope. Whitespace-only is a configured allowlist that
// grants nothing, so it is not this state.
func noScopeAllowlist(appScope sql.NullString) bool {
	return !appScope.Valid || appScope.String == ""
}

// negotiateScope decides the scope the authorization code will carry. Every
// requested name must be in the external scope catalog and covered by the app's
// allowlist. A rejection is an RFC 6749 §4.1.2.1 invalid_scope.
//
//	allowlist  request  result
//	absent     absent   ApiKeyScopeCoderAll, an unrestricted grant
//	absent     present  the request
//	present    absent   the allowlist, catalog-filtered (RFC 6749 §3.3 default)
//	present    present  the request, once shown to be within the allowlist
//
// The result is canonical, deduplicated, and never empty when the error is nil,
// since it is written to a NOT NULL column whose CHECK also rejects "".
func negotiateScope(ctx context.Context, logger slog.Logger, app database.OAuth2ProviderApp, requested []string) (string, error) {
	// Canonicalized first so the catalog check, the coverage comparison, and
	// the stored value all use one spelling. The catalog holds both spellings
	// of the two aliases, so checking after the rewrite accepts the same names.
	granted := canonicalScopes(requested)

	// The catalog is a curation, not a validity check: RBAC also expands
	// internal-only names such as debug_info:read, but clients may not request
	// them.
	for _, s := range granted {
		if !rbac.IsExternalScope(rbac.ScopeName(s)) {
			return "", xerrors.Errorf("%q: %w", s, errUnknownScope)
		}
	}

	if noScopeAllowlist(app.Scope) {
		if len(granted) == 0 {
			// Spelled out because "" would violate the column's CHECK.
			return string(database.ApiKeyScopeCoderAll), nil
		}
		return strings.Join(granted, " "), nil
	}

	// The stored allowlist may name a scope since removed from the catalog, or
	// never in it. Filtering only ever narrows what is granted.
	//
	// Canonicalized in the same pass so both sides expand: rbac.ExpandScope
	// knows `coder:all` and not the `all` alias that IsExternalScope accepts.
	allowed := strings.Fields(app.Scope.String)
	filtered := make([]rbac.ScopeName, 0, len(allowed))
	for _, a := range allowed {
		if name := rbac.ScopeName(a); rbac.IsExternalScope(name) {
			filtered = append(filtered, rbac.CanonicalScopeName(name))
		}
	}
	filtered = slice.Unique(filtered)
	if len(filtered) == 0 {
		// Rejected rather than read as absent, which would grant more than the
		// allowlist ever permitted. The error echoes the stored value so a
		// whitespace-only allowlist does not render as "".
		return "", xerrors.Errorf("%q: %w", app.Scope.String, errNoGrantableScope)
	}

	if len(granted) == 0 {
		names := make([]string, 0, len(filtered))
		for _, name := range filtered {
			names = append(names, string(name))
		}
		return strings.Join(names, " "), nil // RFC 6749 §3.3 default
	}

	// The allowlist is a ceiling on authority, not a menu of spellings, so the
	// check is coverage rather than membership: an app allowed
	// `coder:workspaces.access` can approve a request for `workspace:read`.
	for _, s := range granted {
		covered, err := rbac.ScopesCover(filtered, rbac.ScopeName(s))
		if err != nil {
			// Refuse rather than grant on an incomplete comparison. The
			// underlying error names RBAC internals, so it goes to the log.
			logger.Warn(ctx, "oauth2 scope coverage could not be determined",
				slog.Error(err),
				slog.F("app_id", app.ID.String()),
				slog.F("app_scope", app.Scope.String),
				slog.F("requested_scope", s))
			return "", xerrors.Errorf("%q: %w", s, errCoverageUndecidable)
		}
		if !covered {
			return "", xerrors.Errorf("%q: %w", s, errScopeNotAllowed)
		}
	}
	return strings.Join(granted, " "), nil
}

// consentScopes returns the scope names the consent page lists, and whether the
// grant is unrestricted. An unrestricted grant lists nothing: the page's
// full-access wording tells a user more than "coder:all" does.
func consentScopes(granted string) (names []string, unrestricted bool) {
	names = strings.Fields(granted)
	// Presence, not sole occupancy: an allowlist of
	// `coder:all coder:workspaces.access` defaults to both names.
	if slices.Contains(names, string(database.ApiKeyScopeCoderAll)) {
		return nil, true
	}
	return names, false
}

type authorizeParams struct {
	clientID            string
	redirectURL         *url.URL
	redirectURIProvided bool
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

	// response_type and client_id are always required.
	p.RequiredNotEmpty("response_type", "client_id")

	params := authorizeParams{
		clientID:            p.String(vals, "", "client_id"),
		redirectURL:         p.RedirectURL(vals, callbackURL, "redirect_uri"),
		redirectURIProvided: vals.Get("redirect_uri") != "",
		responseType:        httpapi.ParseCustom(p, vals, "", "response_type", httpapi.ParseEnum[codersdk.OAuth2ProviderResponseType]),
		scope:               strings.Fields(strings.TrimSpace(p.String(vals, "", "scope"))),
		state:               p.String(vals, "", "state"),
		resource:            p.String(vals, "", "resource"),
		codeChallenge:       p.String(vals, "", "code_challenge"),
		codeChallengeMethod: p.String(vals, "", "code_challenge_method"),
	}

	// PKCE is required for the authorization code flow. A malformed
	// code_challenge is rejected here (RFC 7636 §4.4.1) rather than at token
	// exchange, where the error would point at the code_verifier instead.
	if params.responseType == codersdk.OAuth2ProviderResponseTypeCode {
		switch {
		case params.codeChallenge == "":
			p.Errors = append(p.Errors, codersdk.ValidationError{
				Field:  "code_challenge",
				Detail: `Query param "code_challenge" is required and cannot be empty`,
			})
		case !ValidPKCEFormat(params.codeChallenge):
			p.Errors = append(p.Errors, codersdk.ValidationError{
				Field:  "code_challenge",
				Detail: "must be 43 to 128 characters from the unreserved character set [A-Za-z0-9-._~]",
			})
		}
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

// redirectAuthorizeError reports an authorization error through the client's
// own callback, as RFC 6749 §4.1.2.1 requires once the client is known. Only
// callers after extractAuthorizeParams may use it: before that point the
// redirect URI is whatever the request supplied, not the app's registered
// callback.
func redirectAuthorizeError(rw http.ResponseWriter, r *http.Request, redirectURL *url.URL, state string, code codersdk.OAuth2ErrorCode, description string) {
	// Copied because the caller's URL is also the consent page's cancel link
	// and, on the POST side, the success redirect.
	errorURL := *redirectURL
	query := errorURL.Query()
	query.Set("error", string(code))
	query.Set("error_description", description)
	// §4.1.2.1 returns state only when the client sent one.
	if state != "" {
		query.Set("state", state)
	}
	errorURL.RawQuery = query.Encode()

	// 302 rather than 307, matching the success redirect below: some external
	// OAuth2 apps and browsers do not handle 307.
	http.Redirect(rw, r, errorURL.String(), http.StatusFound)
}

// logCorruptCallback reports a registered callback URL this server should never
// have stored: unparsable, or using a scheme registration rejects. The response
// only says the callback is bad, so operators need the log to identify the app.
func logCorruptCallback(ctx context.Context, logger slog.Logger, app database.OAuth2ProviderApp, err error) {
	logger.Error(ctx, "oauth2 app has an unusable registered callback URL",
		slog.Error(err),
		slog.F("app_id", app.ID.String()),
		slog.F("callback_url", app.CallbackURL))
}

// ShowAuthorizePage handles GET /oauth2/authorize requests to display the HTML authorization page.
func ShowAuthorizePage(accessURL *url.URL, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		app := httpmw.OAuth2ProviderApp(r)
		ua := httpmw.UserAuthorization(r.Context())

		callbackURL, err := url.Parse(app.CallbackURL)
		if err != nil {
			logCorruptCallback(r.Context(), logger, app, err)
			site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
				Status:      http.StatusInternalServerError,
				HideStatus:  false,
				Title:       "Internal Server Error",
				Description: err.Error(),
				Actions: []site.Action{
					{
						URL:  accessURL.String(),
						Text: "Back to site",
					},
				},
			})
			return
		}

		params, validationErrs, err := extractAuthorizeParams(r, callbackURL)
		if err != nil {
			errStr := make([]string, len(validationErrs))
			for i, err := range validationErrs {
				errStr[i] = err.Detail
			}
			site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
				Status:      http.StatusBadRequest,
				HideStatus:  false,
				Title:       "Invalid Query Parameters",
				Description: "One or more query parameters are missing or invalid.",
				Warnings:    errStr,
				Actions: []site.Action{
					{
						URL:  accessURL.String(),
						Text: "Back to site",
					},
				},
			})
			return
		}

		// Checked once here, right after the URI has been matched against the
		// registered callback, because later code writes it into a Location
		// header and into the cancel link. 500, not 400: registration rejects
		// these schemes, so a stored one is bad server state.
		if err := codersdk.ValidateRedirectURIScheme(params.redirectURL); err != nil {
			logCorruptCallback(r.Context(), logger, app, err)
			site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
				Status:      http.StatusInternalServerError,
				HideStatus:  false,
				Title:       "Invalid Callback URL",
				Description: "The application's registered callback URL has an invalid scheme.",
				Actions: []site.Action{
					{
						URL:  accessURL.String(),
						Text: "Back to site",
					},
				},
			})
			return
		}

		if params.responseType != codersdk.OAuth2ProviderResponseTypeCode {
			site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
				Status:      http.StatusBadRequest,
				HideStatus:  false,
				Title:       "Unsupported Response Type",
				Description: "Only response_type=code is supported.",
				Actions: []site.Action{
					{
						URL:  accessURL.String(),
						Text: "Back to site",
					},
				},
			})
			return
		}

		// Negotiated here as well as on POST, so a request that cannot succeed
		// fails before the consent page renders rather than after the user
		// clicks Allow. The result also decides what the page lists.
		grantedScope, err := negotiateScope(r.Context(), logger, app, params.scope)
		if err != nil {
			redirectAuthorizeError(rw, r, params.redirectURL, params.state,
				codersdk.OAuth2ErrorCodeInvalidScope, err.Error())
			return
		}

		cancel := params.redirectURL
		cancelQuery := params.redirectURL.Query()
		// Set, not Add: a registered callback carrying its own state= would
		// otherwise hand the client two values.
		cancelQuery.Set("error", "access_denied")
		cancelQuery.Set("error_description", "The resource owner or authorization server denied the request")
		if params.state != "" {
			cancelQuery.Set("state", params.state)
		}
		cancel.RawQuery = cancelQuery.Encode()

		scopes, unrestricted := consentScopes(grantedScope)
		site.RenderOAuthAllowPage(rw, r, site.RenderOAuthAllowData{
			AppIcon: app.Icon,
			AppName: app.Name,
			// #nosec G203 -- The scheme is validated by
			// codersdk.ValidateRedirectURIScheme after extractAuthorizeParams.
			CancelURI:    htmltemplate.URL(cancel.String()),
			DashboardURL: accessURL.String(),
			CSRFToken:    nosurf.Token(r),
			Username:     ua.FriendlyName,
			Scopes:       scopes,
			Unrestricted: unrestricted,
		})
	}
}

// ProcessAuthorize handles POST /oauth2/authorize requests to process the user's authorization decision
// and generate an authorization code.
func ProcessAuthorize(db database.Store, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		apiKey := httpmw.APIKey(r)
		app := httpmw.OAuth2ProviderApp(r)

		callbackURL, err := url.Parse(app.CallbackURL)
		if err != nil {
			logCorruptCallback(ctx, logger, app, err)
			httpapi.WriteOAuth2Error(r.Context(), rw, http.StatusInternalServerError, codersdk.OAuth2ErrorCodeServerError, "Failed to validate query parameters")
			return
		}

		params, _, err := extractAuthorizeParams(r, callbackURL)
		if err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeInvalidRequest, err.Error())
			return
		}

		// As on the GET side: the scope rejection below and the success redirect
		// at the end both write this URL into a Location header.
		if err := codersdk.ValidateRedirectURIScheme(params.redirectURL); err != nil {
			logCorruptCallback(ctx, logger, app, err)
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError,
				codersdk.OAuth2ErrorCodeServerError,
				"The application's registered callback URL has an invalid scheme")
			return
		}

		// OAuth 2.1 removes the implicit grant. Only
		// authorization code flow is supported.
		if params.responseType != codersdk.OAuth2ProviderResponseTypeCode {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest,
				codersdk.OAuth2ErrorCodeUnsupportedResponseType,
				"Only response_type=code is supported")
			return
		}

		// code_challenge is required (enforced by RequiredNotEmpty above),
		// but default the method to S256 if omitted.
		if params.codeChallengeMethod == "" {
			params.codeChallengeMethod = string(codersdk.OAuth2PKCECodeChallengeMethodS256)
		}
		if err := codersdk.ValidatePKCECodeChallengeMethod(params.codeChallengeMethod); err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeInvalidRequest, err.Error())
			return
		}

		grantedScope, err := negotiateScope(ctx, logger, app, params.scope)
		if err != nil {
			redirectAuthorizeError(rw, r, params.redirectURL, params.state,
				codersdk.OAuth2ErrorCodeInvalidScope, err.Error())
			return
		}

		code, err := GenerateSecret()
		if err != nil {
			httpapi.WriteOAuth2Error(r.Context(), rw, http.StatusInternalServerError, codersdk.OAuth2ErrorCodeServerError, "Failed to generate OAuth2 app authorization code")
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
				HashedSecret:        code.Hashed,
				AppID:               app.ID,
				UserID:              apiKey.UserID,
				ResourceUri:         sql.NullString{String: params.resource, Valid: params.resource != ""},
				CodeChallenge:       sql.NullString{String: params.codeChallenge, Valid: params.codeChallenge != ""},
				CodeChallengeMethod: sql.NullString{String: params.codeChallengeMethod, Valid: params.codeChallengeMethod != ""},
				StateHash:           hashOAuth2State(params.state),
				RedirectUri:         sql.NullString{String: params.redirectURL.String(), Valid: params.redirectURIProvided},
				// The negotiated scope, not the requested one. The exchange
				// copies it onto the token row and the API key it mints, so
				// this bounds the issued token.
				Scope: grantedScope,
			})
			if err != nil {
				return xerrors.Errorf("insert oauth2 authorization code: %w", err)
			}

			return nil
		}, nil)
		if err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError, codersdk.OAuth2ErrorCodeServerError, "Failed to generate OAuth2 authorization code")
			return
		}

		newQuery := params.redirectURL.Query()
		// Set, not Add, for the reason the cancel URI uses it.
		newQuery.Set("code", code.Formatted)
		if params.state != "" {
			newQuery.Set("state", params.state)
		}
		params.redirectURL.RawQuery = newQuery.Encode()

		// (ThomasK33): Use a 302 redirect as some (external) OAuth 2 apps and browsers
		// do not work with the 307.
		http.Redirect(rw, r, params.redirectURL.String(), http.StatusFound)
	}
}

// hashOAuth2State returns a SHA-256 hash of the OAuth2 state parameter. If
// the state is empty, it returns a null string.
func hashOAuth2State(state string) sql.NullString {
	if state == "" {
		return sql.NullString{}
	}
	hash := sha256.Sum256([]byte(state))
	return sql.NullString{
		String: hex.EncodeToString(hash[:]),
		Valid:  true,
	}
}
