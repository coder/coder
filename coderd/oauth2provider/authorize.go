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
//	absent     absent   ApiKeyScopeCoderAll, the pre-enforcement grant
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
	response            authorizeResponse
	redirectURIProvided bool
	responseType        codersdk.OAuth2ProviderResponseType
	scope               []string
	resource            string // RFC 8707 resource indicator
	codeChallenge       string // PKCE code challenge
	codeChallengeMethod string // PKCE challenge method
}

// authorizeFailure is a request that did not parse. Which answer it gets is a
// property of the failure rather than of the order the handler's checks happen
// to run in.
type authorizeFailure struct {
	// validationErrors is every field the parser rejected, reported together.
	validationErrors []codersdk.ValidationError
	// message joins them for the response body.
	message string
	// redirect is where RFC 6749 §4.1.2.1 puts the answer. Its zero value means
	// the answer stays on this server, because the failure names the redirect
	// URI or the client identifier.
	redirect authorizeResponse
	// corruptCallback is set when the app's registered callback is unusable.
	// That is bad server state rather than a client mistake, so it answers 500
	// and stops the request before any parameter is read.
	corruptCallback error
}

func extractAuthorizeParams(r *http.Request, registered *url.URL) (authorizeParams, *authorizeFailure) {
	p := httpapi.NewQueryParamParser()
	vals := r.URL.Query()

	// response_type and client_id are always required.
	p.RequiredNotEmpty("response_type", "client_id")

	response, err := newAuthorizeResponse(p, vals, registered)
	if err != nil {
		return authorizeParams{}, &authorizeFailure{corruptCallback: err}
	}

	params := authorizeParams{
		clientID:            p.String(vals, "", "client_id"),
		response:            response,
		redirectURIProvided: vals.Get("redirect_uri") != "",
		responseType:        httpapi.ParseCustom(p, vals, "", "response_type", httpapi.ParseEnum[codersdk.OAuth2ProviderResponseType]),
		scope:               strings.Fields(strings.TrimSpace(p.String(vals, "", "scope"))),
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
		details := make([]string, len(p.Errors))
		for i, err := range p.Errors {
			details[i] = err.Error()
		}
		failure := &authorizeFailure{
			validationErrors: p.Errors,
			message:          "Invalid query params: " + strings.Join(details, ", "),
		}
		if !blamesClient(p.Errors) {
			failure.redirect = response
		}
		return authorizeParams{}, failure
	}
	return params, nil
}

// blamesClient reports whether these errors name the client identifier, the one
// RFC 6749 §4.1.2.1 carve-out a response can still satisfy: a wrong client_id
// means the registration the callback was matched against may not belong to
// whoever is asking. The other carve-out, a redirect URI at fault, needs no test
// here because the response it produced has nowhere to send.
//
// Unreachable through the query parameter, since httpmw resolves the app before
// either handler runs, but reachable through the §2.3.1 Basic credential that
// may stand in for it.
func blamesClient(errs []codersdk.ValidationError) bool {
	return slices.ContainsFunc(errs, func(e codersdk.ValidationError) bool {
		return e.Field == "client_id"
	})
}

// authorizeResponse names where this request's response goes and what it carries
// back. Building one runs both preconditions a Location header needs, so a
// response holding a callback is what licenses a redirect. The unexported fields
// are a guard, not a proof: no other package can fabricate one; this package
// still can.
type authorizeResponse struct {
	// callback is nil when the request named a redirect URI this server will not
	// send anything to. RFC 6749 §4.1.2.1 keeps that answer on this server.
	callback *url.URL
	state    string
}

// newAuthorizeResponse checks the app's registered callback, exact-matches any
// redirect_uri the client sent against it, and reads the state to echo back.
//
// The scheme is checked on the registered URL rather than on the match's result,
// because p.RedirectURL returns the client's URI when the match fails, and
// answering 500 for a scheme the client chose would blame the app for a request
// it did not make. It is checked before the match so no parse outcome can reach
// a Location header through a scheme nothing verified.
//
// A returned error means the registration itself is unusable, which is server
// state. A mismatch is the client's mistake and joins the other parameter
// failures in p.Errors.
func newAuthorizeResponse(p *httpapi.QueryParamParser, vals url.Values, registered *url.URL) (authorizeResponse, error) {
	if err := codersdk.ValidateRedirectURIScheme(registered); err != nil {
		return authorizeResponse{}, err
	}

	before := len(p.Errors)
	callback := p.RedirectURL(vals, registered, "redirect_uri")
	response := authorizeResponse{state: p.String(vals, "", "state")}
	if len(p.Errors) == before {
		response.callback = callback
	}
	return response, nil
}

// canRedirect reports whether this response has somewhere to be sent.
func (a authorizeResponse) canRedirect() bool {
	return a.callback != nil
}

// String returns the callback, without the query a response adds. Valid only on
// a response that holds one.
func (a authorizeResponse) String() string {
	return a.callback.String()
}

// reservedResponseParams are the response parameters RFC 6749 §4.1.2.1 and
// §4.1.2 define. A registered callback may carry any of them: registration
// checks the scheme and rejects fragments, but says nothing about the query.
var reservedResponseParams = []string{"code", "error", "error_description", "state"}

// withQuery returns a copy of the callback with set applied to its query, plus
// the state §4.1.2.1 returns whenever the client sent one. Copied because the
// callback is also what String reports and what ProcessAuthorize stores.
//
// The registered query is kept (§3.1.2) except for the reserved parameters: a
// registered error= would otherwise ride out on a success response, where a
// client reading error first discards a valid code.
func (a authorizeResponse) withQuery(set func(url.Values)) *url.URL {
	destination := *a.callback
	query := destination.Query()
	for _, param := range reservedResponseParams {
		query.Del(param)
	}
	set(query)
	if a.state != "" {
		query.Set("state", a.state)
	}
	destination.RawQuery = query.Encode()
	return &destination
}

// sanitizeErrorDescription confines a description to the NQSCHAR set RFC 6749
// Appendix A permits in error_description. The rule is on the decoded value, so
// percent-encoding on the wire does not satisfy it.
//
// Descriptions quote the client input that was rejected, so the excluded
// characters are the ones %q emits. Quotes become apostrophes rather than
// vanishing: they show where the offending value starts and ends.
func sanitizeErrorDescription(description string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '"':
			return '\'' // permitted, and reads the same
		case r == '\\':
			return -1 // escapes the quote rewritten above
		case r < 0x20 || r > 0x7E:
			return ' '
		default:
			return r
		}
	}, description)
}

// errorURL returns the callback carrying an RFC 6749 §4.1.2.1 error.
func (a authorizeResponse) errorURL(code codersdk.OAuth2ErrorCode, description string) *url.URL {
	return a.withQuery(func(query url.Values) {
		query.Set("error", string(code))
		query.Set("error_description", sanitizeErrorDescription(description))
	})
}

// codeURL returns the callback carrying the authorization code.
func (a authorizeResponse) codeURL(code string) *url.URL {
	return a.withQuery(func(query url.Values) {
		query.Set("code", code)
	})
}

// redirectAuthorizeError reports an authorization error through the client's own
// callback, as RFC 6749 §4.1.2.1 requires once the client is known. Holding a
// response with a callback is what licenses the redirect.
//
// Logged because the failure leaves in a Location header, which loggermw does
// not record, making it indistinguishable from a successful 302. Info, not
// Warn: these are client errors.
func redirectAuthorizeError(rw http.ResponseWriter, r *http.Request, logger slog.Logger, response authorizeResponse, code codersdk.OAuth2ErrorCode, description string) {
	app := httpmw.OAuth2ProviderApp(r)
	logger.Info(r.Context(), "oauth2 authorization rejected",
		slog.F("app_id", app.ID.String()),
		slog.F("error", string(code)),
		slog.F("error_description", description))

	// 302 rather than 307, matching the success redirect below: some external
	// OAuth2 apps and browsers do not handle 307.
	http.Redirect(rw, r, response.errorURL(code, description).String(), http.StatusFound)
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

		params, failure := extractAuthorizeParams(r, callbackURL)
		if failure != nil {
			// 500, not 400: registration rejects these schemes, so a stored one
			// is bad server state and takes precedence over anything the client
			// got wrong in the same request.
			if failure.corruptCallback != nil {
				logCorruptCallback(r.Context(), logger, app, failure.corruptCallback)
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

			// §4.1.2.1: once the callback has been matched against the app's
			// registration, a parameter failure is a response to the client
			// rather than a page for the user. Without it the app never learns
			// its request failed and waits on an authorization that will not
			// arrive.
			if failure.redirect.canRedirect() {
				redirectAuthorizeError(rw, r, logger, failure.redirect,
					codersdk.OAuth2ErrorCodeInvalidRequest, failure.message)
				return
			}

			errStr := make([]string, len(failure.validationErrors))
			for i, err := range failure.validationErrors {
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

		// OAuth 2.1 removes the implicit grant, and §4.1.2.1 delivers
		// unsupported_response_type through the client's own callback.
		//
		// In the query, not the fragment §4.2.2.1 would use: Coder advertises
		// code alone in response_types_supported, so a client asking for token
		// is misconfigured rather than mid-implicit-flow.
		if params.responseType != codersdk.OAuth2ProviderResponseTypeCode {
			redirectAuthorizeError(rw, r, logger, params.response,
				codersdk.OAuth2ErrorCodeUnsupportedResponseType,
				"Only response_type=code is supported")
			return
		}

		// Checked here as well as on POST for the same reason the scope is
		// negotiated below: the page must not render for a request POST will
		// refuse. Only POST defaults an omitted method, since only POST stores it.
		if err := codersdk.ValidatePKCECodeChallengeMethod(params.codeChallengeMethod); err != nil {
			redirectAuthorizeError(rw, r, logger, params.response,
				codersdk.OAuth2ErrorCodeInvalidRequest, err.Error())
			return
		}

		// Negotiated here as well as on POST, so a request that cannot succeed
		// fails before the consent page renders rather than after the user
		// clicks Allow. The result also decides what the page lists.
		grantedScope, err := negotiateScope(r.Context(), logger, app, params.scope)
		if err != nil {
			redirectAuthorizeError(rw, r, logger, params.response,
				codersdk.OAuth2ErrorCodeInvalidScope, err.Error())
			return
		}

		// Declining is an authorization failure like any other, so the cancel
		// link is the same §4.1.2.1 error URL the redirects above build.
		cancel := params.response.errorURL(
			codersdk.OAuth2ErrorCodeAccessDenied,
			"The resource owner or authorization server denied the request")

		scopes, unrestricted := consentScopes(grantedScope)
		site.RenderOAuthAllowPage(rw, r, site.RenderOAuthAllowData{
			AppIcon: app.Icon,
			AppName: app.Name,
			// #nosec G203 -- newAuthorizeResponse checked the scheme before this
			// URL could exist.
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

		params, failure := extractAuthorizeParams(r, callbackURL)
		if failure != nil {
			// As on the GET side: a rejected registered scheme is server state
			// and outranks the client's own mistakes.
			if failure.corruptCallback != nil {
				logCorruptCallback(ctx, logger, app, failure.corruptCallback)
				httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError,
					codersdk.OAuth2ErrorCodeServerError,
					"The application's registered callback URL has an invalid scheme")
				return
			}
			// As on the GET side: §4.1.2.1 delivers this to the client.
			if failure.redirect.canRedirect() {
				redirectAuthorizeError(rw, r, logger, failure.redirect,
					codersdk.OAuth2ErrorCodeInvalidRequest, failure.message)
				return
			}
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeInvalidRequest, failure.message)
			return
		}

		// As on the GET side: OAuth 2.1 removes the implicit grant.
		if params.responseType != codersdk.OAuth2ProviderResponseTypeCode {
			redirectAuthorizeError(rw, r, logger, params.response,
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
			redirectAuthorizeError(rw, r, logger, params.response,
				codersdk.OAuth2ErrorCodeInvalidRequest, err.Error())
			return
		}

		grantedScope, err := negotiateScope(ctx, logger, app, params.scope)
		if err != nil {
			redirectAuthorizeError(rw, r, logger, params.response,
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
				StateHash:           hashOAuth2State(params.response.state),
				RedirectUri:         sql.NullString{String: params.response.String(), Valid: params.redirectURIProvided},
				// The negotiated scope, not the requested one. The exchange
				// copies it onto the token row but not yet onto the API key it
				// mints, so this records what was agreed, not what is enforced.
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

		// (ThomasK33): Use a 302 redirect as some (external) OAuth 2 apps and browsers
		// do not work with the 307.
		http.Redirect(rw, r, params.response.codeURL(code.Formatted).String(), http.StatusFound)
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
