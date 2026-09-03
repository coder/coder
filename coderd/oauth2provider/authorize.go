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

// scopeFailureResponse maps a negotiateScope rejection to the client's error.
// errCoverageUndecidable is this server failing to compare, not a bad request,
// so it answers server_error (RFC 6749 §4.1.2.1) with a fixed description;
// negotiateScope already logged the detail.
func scopeFailureResponse(err error) (codersdk.OAuth2ErrorCode, string) {
	if errors.Is(err, errCoverageUndecidable) {
		return codersdk.OAuth2ErrorCodeServerError, "The requested scope could not be evaluated"
	}
	return codersdk.OAuth2ErrorCodeInvalidScope, err.Error()
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

// maxErrorDescription bounds error_description: long enough for a human reason,
// short enough for a Location header to survive the proxies in front of it.
const maxErrorDescription = 2048

// responseTypeCode is the only response type this server supports. response_type
// is read as text rather than through the SDK enum so every unsupported value
// takes one path, instead of splitting on whether a Go constant happens to
// exist for it.
const responseTypeCode = string(codersdk.OAuth2ProviderResponseTypeCode)

type authorizeParams struct {
	clientID            string
	response            authorizeResponse
	redirectURIProvided bool
	responseType        string
	scope               []string
	resource            string // RFC 8707 resource indicator
	codeChallenge       string // PKCE code challenge
	codeChallengeMethod string // PKCE challenge method
}

// authorizeFailure is a request that will not produce an authorization code:
// either a parameter was rejected, or the app's registration is unusable. Which
// answer it gets is kind(), not the order a handler's checks happen to run in.
type authorizeFailure struct {
	// validationErrors is every field the parser rejected, reported together.
	validationErrors []codersdk.ValidationError
	// description joins them into the error_description the client receives.
	description string
	// redirect is where RFC 6749 §4.1.2.1 puts the answer. Its zero value means
	// the answer stays on this server, because the failure names the redirect
	// URI or the client identifier.
	redirect authorizeResponse
	// corruptCallback is set when the app's registered callback does not parse
	// or uses a scheme registration rejects. That is bad server state rather
	// than a client mistake, so it answers 500, and it is decided before any
	// parameter is read.
	corruptCallback error
	// code is the OAuth2 error to answer with. Read it through errorCode, which
	// supplies the invalid_request default.
	code codersdk.OAuth2ErrorCode
}

func (f authorizeFailure) errorCode() codersdk.OAuth2ErrorCode {
	if f.code == "" {
		return codersdk.OAuth2ErrorCodeInvalidRequest
	}
	return f.code
}

// failureKind is where a failure is answered. The three are mutually exclusive
// by construction here rather than by the shape of authorizeFailure, so both
// handlers dispatch on this instead of re-deriving the precedence from fields.
type failureKind int

const (
	// failureCorruptRegistration outranks the rest: with the registration
	// unusable there is nothing to redirect to, whatever else the client also
	// got wrong.
	failureCorruptRegistration failureKind = iota
	// failureDeliverToClient is the RFC 6749 §4.1.2.1 default.
	failureDeliverToClient
	// failureAnswerHere is a §4.1.2.1 carve-out: no callback this server will
	// send the answer to.
	failureAnswerHere
)

func (f authorizeFailure) kind() failureKind {
	switch {
	case f.corruptCallback != nil:
		return failureCorruptRegistration
	case f.redirect.canRedirect():
		return failureDeliverToClient
	default:
		return failureAnswerHere
	}
}

func extractAuthorizeParams(r *http.Request, logger slog.Logger, app database.OAuth2ProviderApp) (authorizeParams, *authorizeFailure) {
	p := httpapi.NewQueryParamParser()
	vals := r.URL.Query()

	// response_type and client_id are always required.
	p.RequiredNotEmpty("response_type", "client_id")

	response, err := newAuthorizeResponse(p, vals, app.CallbackURL)
	if err != nil {
		return authorizeParams{}, &authorizeFailure{corruptCallback: err}
	}

	params := authorizeParams{
		clientID:            p.String(vals, "", "client_id"),
		response:            response,
		redirectURIProvided: vals.Get("redirect_uri") != "",
		responseType:        p.String(vals, "", "response_type"),
		scope:               strings.Fields(strings.TrimSpace(p.String(vals, "", "scope"))),
		resource:            p.String(vals, "", "resource"),
		codeChallenge:       p.String(vals, "", "code_challenge"),
		codeChallengeMethod: p.String(vals, "", "code_challenge_method"),
	}

	// PKCE is required for the authorization code flow. A malformed
	// code_challenge is rejected here (RFC 7636 §4.4.1) rather than at token
	// exchange, where the error would point at the code_verifier instead.
	//
	// Only for the code flow: an unsupported response type must reach the
	// handlers as unsupported_response_type rather than be recast here as a
	// missing code_challenge.
	if params.responseType == responseTypeCode {
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

	// RFC 6749 §3.1 and OAuth 2.1 §3.1: unrecognized parameters MUST be ignored,
	// so an OIDC nonce or a vendor extension is not this endpoint's business.
	// Repeats of the parameters read above are still rejected, by parseSingle.
	if ignored := ignoredParams(p, vals); len(ignored) > 0 {
		logger.Debug(r.Context(), "ignoring unrecognized authorization parameters",
			slog.F("params", ignored))
	}

	if len(p.Errors) > 0 {
		// Not err.Error(): its "field: x detail: y" shape is a Coder debug
		// formatter, and details contain commas, so a comma join cannot be split
		// back into per-field diagnostics by the client reading it.
		details := make([]string, len(p.Errors))
		for i, err := range p.Errors {
			details[i] = err.Field + ": " + err.Detail
		}
		failure := &authorizeFailure{
			validationErrors: p.Errors,
			description:      "Invalid query params: " + strings.Join(details, "; "),
		}
		// RFC 8707 §2 gives resource its own code, but only when nothing else
		// failed. A client that retries on invalid_target would otherwise resend
		// a request that is still broken in the field it did not hear about.
		if !slices.ContainsFunc(p.Errors, func(e codersdk.ValidationError) bool {
			return e.Field != "resource"
		}) {
			failure.code = codersdk.OAuth2ErrorCodeInvalidTarget
		}
		if !clientIDInDoubt(vals, params.clientID, app.ID) {
			failure.redirect = response
		}
		return authorizeParams{}, failure
	}
	return params, nil
}

// ignoredParams returns the query parameters this endpoint does not read,
// sorted so the log line is stable. A misspelled parameter (redirect_url for
// redirect_uri) surfaces here instead of in the client's error.
func ignoredParams(p *httpapi.QueryParamParser, vals url.Values) []string {
	var ignored []string
	for name := range vals {
		if !p.Parsed[name] {
			ignored = append(ignored, name)
		}
	}
	slices.Sort(ignored)
	return ignored
}

// clientIDInDoubt reports whether the client's identity is unsettled, the
// RFC 6749 §4.1.2.1 carve-out that keeps the answer on this server rather than
// sending it to a registration that may not be the caller's. The other
// carve-out, a redirect URI at fault, needs no test here because the response
// it produced has nowhere to send.
//
// It reads the raw values because parseSingle collapses a repeated client_id to
// "", which is indistinguishable from a POST carrying client_id in the form
// body. httpmw accepts that body, so an absent query parameter still names a
// client and its failure is deliverable.
func clientIDInDoubt(vals url.Values, parsed string, appID uuid.UUID) bool {
	// RFC 6749 §3.1: a parameter sent without a value is the omitted case, so
	// ?client_id= names no candidate, and neither does a repeat of it.
	named := slices.DeleteFunc(slices.Clone(vals["client_id"]), func(v string) bool {
		return v == ""
	})
	switch {
	case len(named) > 1:
		// The callback was matched against one of several candidates.
		return true
	case len(named) == 0:
		return false
	default:
		// Parsed rather than compared as text: httpmw resolves through
		// uuid.Parse, which accepts spellings the canonical form does not match.
		id, err := uuid.Parse(parsed)
		return err != nil || id != appID
	}
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

// newAuthorizeResponse parses the app's registered callback, checks it,
// exact-matches any redirect_uri the client sent against it, and reads the
// state to echo back.
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
func newAuthorizeResponse(p *httpapi.QueryParamParser, vals url.Values, registered string) (authorizeResponse, error) {
	registeredURL, err := url.Parse(registered)
	if err != nil {
		return authorizeResponse{}, err
	}
	if err := codersdk.ValidateRedirectURIScheme(registeredURL); err != nil {
		return authorizeResponse{}, err
	}

	callback := p.RedirectURL(vals, registeredURL, "redirect_uri")
	response := authorizeResponse{state: p.String(vals, "", "state")}
	// The field, not a count of errors across these two lines: reading state
	// can fail too, and that failure belongs to the client's callback rather
	// than to the carve-out that withholds one.
	if !slices.ContainsFunc(p.Errors, func(e codersdk.ValidationError) bool {
		return e.Field == "redirect_uri"
	}) {
		response.callback = callback
	}
	return response, nil
}

func (a authorizeResponse) canRedirect() bool {
	return a.callback != nil
}

// callbackURL returns the destination, without the query a response adds. Named
// rather than String so the type is not an implicit fmt.Stringer: the zero value
// is routine on failure paths, and its String would panic through %v. Valid only
// on a response that holds a callback.
func (a authorizeResponse) callbackURL() string {
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

func redirectAuthorizeError(rw http.ResponseWriter, r *http.Request, logger slog.Logger, response authorizeResponse, code codersdk.OAuth2ErrorCode, description string) {
	// Descriptions echo values the client sent, so their length is the client's
	// to choose. Cap here, ahead of both the log field and the Location header.
	if len(description) > maxErrorDescription {
		description = description[:maxErrorDescription] + " (truncated)"
	}

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

		errorPage := func(status int, title, description string, warnings []string) {
			site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
				Status:      status,
				HideStatus:  false,
				Title:       title,
				Description: description,
				Warnings:    warnings,
				Actions: []site.Action{
					{
						URL:  accessURL.String(),
						Text: "Back to site",
					},
				},
			})
		}

		params, failure := extractAuthorizeParams(r, logger, app)
		if failure != nil {
			switch failure.kind() {
			case failureCorruptRegistration:
				logCorruptCallback(r.Context(), logger, app, failure.corruptCallback)
				errorPage(http.StatusInternalServerError, "Invalid Callback URL",
					"The application's registered callback URL is not usable.", nil)

			case failureDeliverToClient:
				// §4.1.2.1: once the callback has been matched against the app's
				// registration, a parameter failure is a response to the client
				// rather than a page for the user. Without it the app never
				// learns its request failed and waits on an authorization that
				// will not arrive.
				redirectAuthorizeError(rw, r, logger, failure.redirect,
					failure.errorCode(), failure.description)

			case failureAnswerHere:
				warnings := make([]string, len(failure.validationErrors))
				for i, err := range failure.validationErrors {
					warnings[i] = err.Detail
				}
				errorPage(http.StatusBadRequest, "Invalid Query Parameters",
					"One or more query parameters are missing or invalid.", warnings)

			default:
				logger.Error(r.Context(), "unhandled authorize failure kind",
					slog.F("kind", int(failure.kind())))
				errorPage(http.StatusInternalServerError, "Internal Server Error",
					"The request could not be answered.", nil)
			}
			return
		}

		// OAuth 2.1 removes the implicit grant, and §4.1.2.1 delivers
		// unsupported_response_type through the client's own callback.
		//
		// In the query, not the fragment §4.2.2.1 would use: Coder advertises
		// code alone in response_types_supported, so a client asking for token
		// is misconfigured rather than mid-implicit-flow.
		if params.responseType != responseTypeCode {
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
			code, description := scopeFailureResponse(err)
			redirectAuthorizeError(rw, r, logger, params.response, code, description)
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

		params, failure := extractAuthorizeParams(r, logger, app)
		if failure != nil {
			switch failure.kind() {
			case failureCorruptRegistration:
				logCorruptCallback(ctx, logger, app, failure.corruptCallback)
				httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError,
					codersdk.OAuth2ErrorCodeServerError,
					"The application's registered callback URL is not usable")

			case failureDeliverToClient:
				redirectAuthorizeError(rw, r, logger, failure.redirect,
					failure.errorCode(), failure.description)

			case failureAnswerHere:
				httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, failure.errorCode(), failure.description)

			default:
				logger.Error(ctx, "unhandled authorize failure kind",
					slog.F("kind", int(failure.kind())))
				httpapi.WriteOAuth2Error(ctx, rw, http.StatusInternalServerError,
					codersdk.OAuth2ErrorCodeServerError, "The request could not be answered")
			}
			return
		}

		// As on the GET side: OAuth 2.1 removes the implicit grant.
		if params.responseType != responseTypeCode {
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
			code, description := scopeFailureResponse(err)
			redirectAuthorizeError(rw, r, logger, params.response, code, description)
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
				RedirectUri:         sql.NullString{String: params.response.callbackURL(), Valid: params.redirectURIProvided},
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
