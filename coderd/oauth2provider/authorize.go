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

// Rejection reasons from negotiateScope. They are sentinels rather than inline
// messages so a caller, and the tests, can tell which check failed without
// matching on message text.
//
// Each is wrapped with the offending value ahead of it, because xerrors only
// wraps without repeating the sentinel's own text when %w is the final verb.
// These messages are rendered into error_description, so a doubled one is read
// by a person.
var (
	// errUnknownScope is returned for a scope name outside the external scope
	// catalog, whether unrecognized entirely or recognized but internal-only.
	errUnknownScope = xerrors.New("unknown or unsupported scope")
	// errNoGrantableScope is returned when every entry of the app's allowlist
	// falls outside the catalog, leaving nothing the app can be granted. The
	// request is not at fault here and may have carried no scope at all, so
	// the message names the registered list and points at the remedy without
	// prescribing a route to it: an admin edits the app, and a dynamically
	// registered client updates itself through RFC 7592.
	errNoGrantableScope = xerrors.New("none of the scopes registered for this app are supported by this deployment; change the app's registered scopes to supported ones")
	// errScopeNotAllowed is returned for a catalog scope whose permissions the
	// app's allowlist does not cover. Phrased as coverage rather than list
	// membership, because a scope absent from the allowlist by name is still
	// granted when a listed composite already confers it.
	errScopeNotAllowed = xerrors.New("scope requests permissions beyond this app's allowed scopes")
	// errCoverageUndecidable is returned when the allowlist and the request
	// cannot be compared at all. That is a deployment-side condition, not
	// something the client can correct by asking differently, and the
	// comparison's own error names RBAC internals, so it is logged rather
	// than rendered into error_description.
	errCoverageUndecidable = xerrors.New("scope coverage against this app's allowed scopes could not be determined")
)

// canonicalScopes rewrites each name to the spelling the api_key_scope enum
// stores and drops repeats, preserving the order of first appearance.
//
// It neither validates nor filters: callers check rbac.IsExternalScope
// separately. Canonicalization is required because rbac.IsExternalScope
// accepts the aliases `all` and `application_connect`, which are not enum
// members, so persisting a validated name verbatim can write a value the
// column's vocabulary does not contain. Deduplicating here keeps the stored
// value set-valued, which is what a space-separated scope denotes.
func canonicalScopes(names []string) []string {
	canonical := make([]string, 0, len(names))
	for _, name := range names {
		canonical = append(canonical, string(rbac.CanonicalScopeName(rbac.ScopeName(name))))
	}
	return slice.Unique(canonical)
}

// noScopeAllowlist reports whether an app has no scope allowlist configured.
// NULL and "" are one state, and this is the only place the two are unified:
// admin-created apps store sql.NullString{} (apps.go), while DCR-registered
// apps store Valid: true carrying a possibly-empty req.Scope
// (registration.go). Once the allowlist decides what a token may do, reading
// it is an authorization decision, so the two encodings route through one
// predicate rather than each caller flattening via .String.
//
// A whitespace-only allowlist is deliberately not this state. It is a
// configured value that grants nothing, so it falls through to
// negotiateScope's filtered-to-empty rejection instead of the unrestricted
// fallback.
func noScopeAllowlist(appScope sql.NullString) bool {
	return !appScope.Valid || appScope.String == ""
}

// negotiateScope decides the scope the authorization code will carry. Every
// requested name must be in the external scope catalog (RFC 6749 §4.1.2.1
// invalid_scope), and the request must be covered by the app's configured
// allowlist.
//
// What each branch returns:
//
//	allowlist  request  result
//	absent     absent   ApiKeyScopeCoderAll, the pre-enforcement grant
//	absent     present  the request, which is narrower than unrestricted
//	present    absent   the allowlist, catalog-filtered (RFC 6749 §3.3 default)
//	present    present  the request, once shown to be within the allowlist
//
// An allowlist is absent when NULL or empty, which noScopeAllowlist treats as
// one state. An allowlist whose every entry falls outside the catalog is
// rejected rather than read as absent, since falling back there would grant
// strictly more than the allowlist ever permitted.
//
// The return value is written directly to a NOT NULL column whose CHECK
// constraint also rejects the empty string, so it is a string rather than a
// []string, and it is never empty alongside a nil error. Its names are
// canonical api_key_scope spellings and carry no duplicates, so the value can
// be stored as that enum without further rewriting.
//
// The whole app is taken rather than just its scope because a coverage failure
// is a deployment-side fault, and the log line that records it is only useful
// if it names the app that provoked it.
func negotiateScope(ctx context.Context, logger slog.Logger, app database.OAuth2ProviderApp, requested []string) (string, error) {
	// Only names in the external scope catalog (rbac.IsExternalScope) are
	// user-requestable. That is a curation, not a validity check: RBAC can
	// expand internal-only names such as debug_info:read just fine, and the
	// api_key_scope enum would store them, which is exactly why the catalog
	// exists as a narrower list. Checking here keeps both an unrecognizable
	// name and an internal-only one out of the granted scope, whether or not
	// the app has an allowlist to check against.
	for _, s := range requested {
		if !rbac.IsExternalScope(rbac.ScopeName(s)) {
			return "", xerrors.Errorf("%q: %w", s, errUnknownScope)
		}
	}

	// Canonicalized after the catalog check, so a rejection names the scope
	// as the client spelled it rather than as the server stores it.
	granted := canonicalScopes(requested)

	if noScopeAllowlist(app.Scope) {
		if len(requested) == 0 {
			// Unrestricted, the same grant this app got before scope
			// enforcement existed, but stated explicitly: an empty string
			// would violate the column's CHECK.
			return string(database.ApiKeyScopeCoderAll), nil
		}
		return strings.Join(granted, " "), nil
	}

	// Filter the allowlist through IsExternalScope before it is used for
	// anything. The allowlist was stored at registration time and may contain
	// a scope name since removed from the curated catalog, or never in it at
	// all. Filtering only ever narrows what is granted.
	allowed := strings.Fields(app.Scope.String)
	filtered := make([]string, 0, len(allowed))
	for _, a := range allowed {
		if rbac.IsExternalScope(rbac.ScopeName(a)) {
			filtered = append(filtered, a)
		}
	}
	if len(filtered) == 0 {
		// The app has an allowlist, but no entry in it is grantable.
		// Returning the unrestricted sentinel here would grant strictly more
		// than the allowlist ever permitted, so reject instead. This is the
		// all-entries-dropped counterpart to the single-stale-entry case the
		// filter above handles, and it must not share the no-allowlist
		// branch's fallback.
		//
		// Named with the stored value verbatim, since that is what was
		// registered and what the app owner has to change. Rejoining the
		// filter's input instead would render a whitespace-only allowlist as
		// "", naming nothing for the one configuration that most needs it.
		return "", xerrors.Errorf("%q: %w", app.Scope.String, errNoGrantableScope)
	}
	// Canonicalized so both sides expand: rbac.ExpandScope knows `coder:all`
	// and not the `all` alias that IsExternalScope accepts.
	filtered = canonicalScopes(filtered)

	if len(requested) == 0 {
		return strings.Join(filtered, " "), nil // RFC 6749 §3.3 default
	}

	// The allowlist is a ceiling on authority, not a menu of spellings, so the
	// check is permission coverage rather than name membership. An app allowed
	// `coder:workspaces.access` can approve a client asking only for
	// `workspace:read`, which the composite already grants; under name
	// matching that client's only route to a token was to request the broader
	// composite instead. Coverage runs against the filtered allowlist, not the
	// raw one, so a dropped entry grants nothing.
	allowedNames := make([]rbac.ScopeName, 0, len(filtered))
	for _, a := range filtered {
		allowedNames = append(allowedNames, rbac.ScopeName(a))
	}
	for _, s := range granted {
		covered, err := rbac.ScopesCover(allowedNames, rbac.ScopeName(s))
		if err != nil {
			// Coverage could not be decided, so the request is refused rather
			// than granted on an incomplete comparison. The comparison's own
			// error names RBAC internals the client can do nothing with, so it
			// goes to the log alongside the app that provoked it, and only the
			// sentinel reaches error_description.
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

// consentScopes returns the scope names the consent page lists, or nil when the
// grant is unrestricted, since "coder:all" states to a user far less than the
// page's own full-access wording does.
//
// The negotiated value is canonical and deduplicated by the time it arrives
// here, so this splits rather than rewrites.
func consentScopes(granted string) []string {
	names := strings.Fields(granted)
	// Presence, not sole occupancy: an allowlist registered as
	// `coder:all coder:workspaces.access` defaults to both names, and listing
	// them would show the user `coder:all` while understating a grant that is
	// in fact unrestricted.
	if slices.Contains(names, string(database.ApiKeyScopeCoderAll)) {
		return nil
	}
	return names
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

	// PKCE is required for authorization code flow requests. Reject a
	// malformed code_challenge here (RFC 7636 §4.4.1) rather than storing it
	// verbatim and failing later at token exchange, where the error would
	// point at the code_verifier instead of the parameter that was actually
	// invalid.
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

// redirectAuthorizeError returns an authorization error to the client by
// redirecting to its callback with the error in the query, which is how
// RFC 6749 §4.1.2.1 says an authorization request fails once the client is
// known. Delivering it on Coder instead reaches only the user's screen: the
// client's error handling never runs, and the state it sent is dropped, so it
// cannot correlate the failure with the request that caused it.
//
// Only errors raised after extractAuthorizeParams returns may use this. Before
// that point the redirect URI is whatever the request supplied, and §4.1.2.1
// requires informing the user rather than redirecting to it. Afterwards it has
// been exact-matched against the app's registered callback, so the destination
// is the app's own no matter what the request carried.
func redirectAuthorizeError(rw http.ResponseWriter, r *http.Request, redirectURL *url.URL, state string, code codersdk.OAuth2ErrorCode, description string) {
	// Copied because the caller's URL is also the consent page's cancel link
	// and, on the POST side, the success redirect.
	errorURL := *redirectURL
	query := errorURL.Query()
	query.Set("error", string(code))
	query.Set("error_description", description)
	// RFC 6749 §4.1.2.1 requires the state back exactly as it arrived,
	// whenever the client sent one.
	if state != "" {
		query.Set("state", state)
	}
	errorURL.RawQuery = query.Encode()

	// 302 rather than 307, matching the success redirect below: some external
	// OAuth2 apps and browsers do not handle 307.
	http.Redirect(rw, r, errorURL.String(), http.StatusFound)
}

// ShowAuthorizePage handles GET /oauth2/authorize requests to display the HTML authorization page.
func ShowAuthorizePage(accessURL *url.URL, logger slog.Logger) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		app := httpmw.OAuth2ProviderApp(r)
		ua := httpmw.UserAuthorization(r.Context())

		callbackURL, err := url.Parse(app.CallbackURL)
		if err != nil {
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

		// Everything downstream writes this URL somewhere a scheme matters: the
		// error redirects into a Location header, the cancel link into an href.
		// Checking once here, immediately after the URI has been exact-matched
		// against the app's registered callback, is what makes those writes safe.
		// Checking at each write instead leaves the next one to remember.
		if err := codersdk.ValidateRedirectURIScheme(params.redirectURL); err != nil {
			site.RenderStaticErrorPage(rw, r, site.ErrorPageData{
				Status:      http.StatusBadRequest,
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

		// Reject a scope the app can never be granted before the consent page
		// renders, rather than after the user clicks Allow. Both handlers run
		// the check for that reason: this one to decide what the page states
		// and whether it renders at all, the POST side to persist it. The two
		// negotiate the same query string, since the consent form posts back
		// to this URL.
		grantedScope, err := negotiateScope(r.Context(), logger, app, params.scope)
		if err != nil {
			redirectAuthorizeError(rw, r, params.redirectURL, params.state,
				codersdk.OAuth2ErrorCodeInvalidScope, err.Error())
			return
		}

		cancel := params.redirectURL
		cancelQuery := params.redirectURL.Query()
		// Set, not Add: a registered callback carrying its own state= would
		// otherwise hand the client two values from here and one from the error
		// path, and a client is entitled to reject that as malformed.
		cancelQuery.Set("error", "access_denied")
		cancelQuery.Set("error_description", "The resource owner or authorization server denied the request")
		if params.state != "" {
			cancelQuery.Set("state", params.state)
		}
		cancel.RawQuery = cancelQuery.Encode()

		site.RenderOAuthAllowPage(rw, r, site.RenderOAuthAllowData{
			AppIcon: app.Icon,
			AppName: app.Name,
			// #nosec G203 -- The scheme is validated by
			// codersdk.ValidateRedirectURIScheme after extractAuthorizeParams.
			CancelURI:    htmltemplate.URL(cancel.String()),
			DashboardURL: accessURL.String(),
			CSRFToken:    nosurf.Token(r),
			Username:     ua.FriendlyName,
			Scopes:       consentScopes(grantedScope),
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
			httpapi.WriteOAuth2Error(r.Context(), rw, http.StatusInternalServerError, codersdk.OAuth2ErrorCodeServerError, "Failed to validate query parameters")
			return
		}

		params, _, err := extractAuthorizeParams(r, callbackURL)
		if err != nil {
			httpapi.WriteOAuth2Error(ctx, rw, http.StatusBadRequest, codersdk.OAuth2ErrorCodeInvalidRequest, err.Error())
			return
		}

		// The same guarantee the GET side establishes: the scope rejection below
		// and the success redirect at the end both write this URL into a
		// Location header, so the scheme is checked once here rather than at
		// each write. A registered callback reaching this point with a
		// dangerous scheme is bad server state, not a bad request, since
		// registration rejects those schemes.
		if err := codersdk.ValidateRedirectURIScheme(params.redirectURL); err != nil {
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
				// The negotiated scope, not the requested one: it has been
				// checked against the scope catalog and the app's allowlist.
				// The exchange copies it onto the token row but does not yet
				// put it on the API key it mints, so what is recorded here is
				// what was agreed, not yet what is enforced.
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
