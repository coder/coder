package oauth2provider_test

import (
	"context"
	"database/sql"
	htmltemplate "html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/oauth2provider"
	"github.com/coder/coder/v2/coderd/oauth2provider/oauth2providertest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/site"
	"github.com/coder/coder/v2/testutil"
)

func TestOAuthConsentFormIncludesCSRFToken(t *testing.T) {
	t.Parallel()

	const csrfFieldValue = "csrf-field-value"
	req := httptest.NewRequest(http.MethodGet, "https://coder.com/oauth2/authorize", nil)
	rec := httptest.NewRecorder()

	site.RenderOAuthAllowPage(rec, req, site.RenderOAuthAllowData{
		AppName:      "Test OAuth App",
		CancelURI:    htmltemplate.URL("https://coder.com/cancel"),
		DashboardURL: "https://coder.com/",
		CSRFToken:    csrfFieldValue,
		Username:     "test-user",
		// The page refuses to render a grant carrying no permission.
		Scopes: []string{"workspace:ssh"},
	})

	require.Equal(t, http.StatusOK, rec.Result().StatusCode)
	body := rec.Body.String()
	assert.Contains(t, body, `name="csrf_token"`)
	assert.Contains(t, body, `value="`+csrfFieldValue+`"`)
	assert.Contains(t, body, `id="allow-form"`)
	assert.Contains(t, body, `id="cancel-link"`)
}

// The consent page is the only place a person is told what they are about to
// approve. A narrow grant must not read as full access, and a full grant must
// not be named by a scope no user would recognize.
func TestOAuthConsentFormStatesNegotiatedScope(t *testing.T) {
	t.Parallel()

	record := func(t *testing.T, scopes []string, unrestricted bool) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "https://coder.com/oauth2/authorize", nil)
		rec := httptest.NewRecorder()
		site.RenderOAuthAllowPage(rec, req, site.RenderOAuthAllowData{
			AppName:      "Test OAuth App",
			CancelURI:    htmltemplate.URL("https://coder.com/cancel"),
			DashboardURL: "https://coder.com/",
			CSRFToken:    "csrf-field-value",
			Username:     "test-user",
			Scopes:       scopes,
			Unrestricted: unrestricted,
		})
		return rec
	}

	render := func(t *testing.T, scopes []string, unrestricted bool) string {
		t.Helper()
		rec := record(t, scopes, unrestricted)
		require.Equal(t, http.StatusOK, rec.Result().StatusCode)
		return rec.Body.String()
	}

	t.Run("NarrowScopeListed", func(t *testing.T) {
		t.Parallel()

		body := render(t, []string{"workspace:ssh", "template:read"}, false)
		assert.Contains(t, body, "workspace:ssh")
		assert.Contains(t, body, "template:read")
		assert.NotContains(t, body, "full access",
			"a scoped grant must not be described as full access")
		// Both roles read as redundant markup, but WebKit drops implicit list
		// semantics under `list-style: none`.
		assert.Contains(t, body, `role="list"`)
		assert.Contains(t, body, `role="listitem"`)
		// The id is the handle the submit and cancel handlers hide the list by,
		// so a rename leaves the permissions on screen under "is now
		// authorized".
		assert.Contains(t, body, `id="scope-list"`)
		assert.Contains(t, body, `id="scope-disclaimer"`)
		// The added branch must not cost the page its approval controls.
		assert.Contains(t, body, `id="allow-form"`)
		assert.Contains(t, body, `id="cancel-link"`)
	})

	t.Run("UnrestrictedStaysFullAccess", func(t *testing.T) {
		t.Parallel()

		body := render(t, nil, true)
		assert.Contains(t, body, "full access")
		assert.NotContains(t, body, `id="scope-list"`)
		// The disclaimer has nothing to qualify on a page that lists nothing.
		assert.NotContains(t, body, `id="scope-disclaimer"`)
	})

	// A grant carrying no permission is the opposite of an unrestricted one, so
	// falling back to the length of Scopes would describe the narrowest grant
	// there is as full access. The page refuses it rather than asking anyone to
	// approve "these permissions" above an empty list. No caller can reach this
	// today; a future one computing the grant itself is what the guard is for.
	t.Run("EmptyScopesAreRefused", func(t *testing.T) {
		t.Parallel()

		rec := record(t, []string{}, false)
		require.Equal(t, http.StatusInternalServerError, rec.Result().StatusCode)
		body := rec.Body.String()
		assert.NotContains(t, body, "full access")
		assert.NotContains(t, body, `id="allow-form"`)
		assert.NotContains(t, body, `id="scope-list"`)
	})

	// nil and an empty slice are the same grant, and a guard written against
	// one spelling would let the other through.
	t.Run("NilScopesAreRefused", func(t *testing.T) {
		t.Parallel()

		rec := record(t, nil, false)
		require.Equal(t, http.StatusInternalServerError, rec.Result().StatusCode)
		assert.NotContains(t, rec.Body.String(), `id="allow-form"`)
	})
}

const (
	scopeInCatalog     = "coder:workspaces.access"
	scopeAlsoInCatalog = "coder:templates.build"
	scopeOutOfCatalog  = "some_removed_scope"
	// In the catalog, but scopeInCatalog grants template:read, never
	// template:update.
	scopeOutOfAllowlist = "template:update"
)

const (
	appCallbackURL = "https://example.com/callback"
	authorizeState = "test-authorize-state"
)

func TestOAuth2AuthorizeScopeNegotiation(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	// Only one code exists per app/user pair, so each sub-test needs its own app.
	seedApp := func(t *testing.T, appScope sql.NullString) database.OAuth2ProviderApp {
		t.Helper()
		return dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
			Name:        testutil.GetRandomName(t),
			CallbackURL: appCallbackURL,
			Scope:       appScope,
		})
	}

	t.Run("OutOfAllowlistRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), scopeInCatalog+" "+scopeOutOfAllowlist)
		defer resp.Body.Close()

		requireInvalidScope(t, resp, reasonScopeNotAllowed)
	})

	t.Run("UnlistedScopeCoveredByAllowlistGranted", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "workspace:ssh")
		defer resp.Body.Close()

		require.Equal(t, "workspace:ssh", persistedCodeScope(ctx, t, db, resp))
	})

	t.Run("UnknownScopeRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "not_a_real_scope")
		defer resp.Body.Close()

		requireInvalidScope(t, resp, reasonUnknownScope)
	})

	t.Run("OmittedScopeDefaultsToAllowlist", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		allowlist := scopeInCatalog + " " + scopeAlsoInCatalog
		app := seedApp(t, sql.NullString{String: allowlist, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "")
		defer resp.Body.Close()

		require.Equal(t, allowlist, persistedCodeScope(ctx, t, db, resp))
	})

	t.Run("LegacyAliasPersistedCanonically", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "all")
		defer resp.Body.Close()

		require.Equal(t, string(database.ApiKeyScopeCoderAll), persistedCodeScope(ctx, t, db, resp))
	})

	t.Run("DuplicateRequestedScopePersistedOnce", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), scopeInCatalog+" "+scopeInCatalog)
		defer resp.Body.Close()

		require.Equal(t, scopeInCatalog, persistedCodeScope(ctx, t, db, resp))
	})

	// NULL comes from admin-created apps, '' from DCR apps that sent no scope.
	t.Run("NullAndEmptyAllowlistGrantUnrestricted", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		nullApp := seedApp(t, sql.NullString{})
		emptyApp := seedApp(t, sql.NullString{String: "", Valid: true})

		nullResp := authorizeRequest(ctx, t, client, http.MethodPost, nullApp.ID.String(), "")
		defer nullResp.Body.Close()
		emptyResp := authorizeRequest(ctx, t, client, http.MethodPost, emptyApp.ID.String(), "")
		defer emptyResp.Body.Close()

		nullScope := persistedCodeScope(ctx, t, db, nullResp)
		emptyScope := persistedCodeScope(ctx, t, db, emptyResp)
		require.Equal(t, string(database.ApiKeyScopeCoderAll), nullScope)
		require.Equal(t, nullScope, emptyScope)
	})

	t.Run("StaleAllowlistEntryDroppedNotGranted", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog + " " + scopeOutOfCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "")
		defer resp.Body.Close()

		require.Equal(t, scopeInCatalog, persistedCodeScope(ctx, t, db, resp))
	})

	t.Run("AllowlistFilteringToEmptyRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: "openid profile email", Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "")
		defer resp.Body.Close()

		requireInvalidScope(t, resp, reasonNoGrantableScope)
	})

	// The GET handler rejects before the consent page renders, so the user is
	// never asked to approve a request that cannot succeed.
	t.Run("ConsentPageNotRenderedForInvalidScope", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})

		resp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), scopeInCatalog+" "+scopeOutOfAllowlist)
		defer resp.Body.Close()
		requireInvalidScope(t, resp, reasonScopeNotAllowed)
	})

	// The wiring rather than the template: the page a user is served must name
	// the scope the code will carry.
	t.Run("ConsentPageStatesNegotiatedScope", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})

		resp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), "workspace:ssh")
		defer resp.Body.Close()

		body := readBody(t, resp)
		require.Contains(t, body, `id="allow-form"`, "the consent page must render")
		require.Contains(t, body, "workspace:ssh")
		require.NotContains(t, body, "full access",
			"a scoped grant must not be described as full access")
		// The allowlist covers workspace:ssh and more, so listing it would
		// satisfy every assertion above while overstating the grant.
		require.NotContains(t, body, scopeInCatalog,
			"the consent page must state the negotiated scope, not the app's allowlist")
	})

	// The unrestricted half of the same wiring. The collapse and the template's
	// full-access branch are covered alone, so dropping the collapse would show
	// a real user `coder:all` with every other test still green.
	t.Run("ConsentPageStatesFullAccessWhenUnrestricted", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{})

		resp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), "")
		defer resp.Body.Close()

		body := readBody(t, resp)
		require.Contains(t, body, `id="allow-form"`, "the consent page must render")
		require.Contains(t, body, "full access")
		require.NotContains(t, body, string(database.ApiKeyScopeCoderAll),
			"an unrestricted grant must not be stated to a user as a scope name")
	})

	// RFC 6749 §4.1.2.1 returns state only if the request carried one, and an
	// empty state is not the same as no state. Every other case here sends one,
	// so the guard could be deleted with the suite staying green.
	t.Run("OmittedStateNotEchoed", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})
		query := authorizeQuery(t, app.ID.String(), "not_a_real_scope")
		query.Del("state")

		resp := sendAuthorizeRequest(ctx, t, client, http.MethodGet, query)
		defer resp.Body.Close()

		require.Equal(t, http.StatusFound, resp.StatusCode)
		location, err := url.Parse(resp.Header.Get("Location"))
		require.NoError(t, err)
		// Pinned so the case cannot pass on a redirect that failed ahead of the
		// state guard.
		require.Equal(t, string(codersdk.OAuth2ErrorCodeInvalidScope), location.Query().Get("error"))
		require.False(t, location.Query().Has("state"),
			"a client that sent no state must not receive an empty one")
	})

	// The other half of RFC 6749 §4.1.2.1: an unregistered redirect URI is never
	// a destination this server sends anyone to, however the request fails.
	// That validation running first is what keeps the rejection redirect above
	// from being reachable with a request-supplied URI.
	t.Run("MismatchedRedirectURINotRedirected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})

		for _, method := range []string{http.MethodGet, http.MethodPost} {
			query := authorizeQuery(t, app.ID.String(), "not_a_real_scope")
			query.Set("redirect_uri", "https://not-the-registered-callback.example/cb")

			resp := sendAuthorizeRequest(ctx, t, client, method, query)
			defer resp.Body.Close()

			require.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"%s: an unregistered redirect_uri must fail on Coder", method)
			require.Empty(t, resp.Header.Get("Location"),
				"%s: the user must not be redirected to a URI the app did not register", method)
			// The request also carries an invalid scope, so this pins that the
			// redirect URI is what rejected it first.
			require.Contains(t, readBody(t, resp), "must exactly match",
				"%s: the rejection must come from redirect_uri validation", method)
		}

		// Positive control: the same handler still renders the consent page for
		// a request the app can be granted.
		okResp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), scopeInCatalog)
		defer okResp.Body.Close()
		require.Equal(t, http.StatusOK, okResp.StatusCode)
		require.Contains(t, readBody(t, okResp), `id="allow-form"`)
	})

	// A registered callback whose scheme is dangerous in a browser is refused
	// before anything writes it: no Location header, and on GET no cancel link
	// either. The request also carries a scope the app cannot be granted, so
	// the rejection redirect is the write that would otherwise fire, which is
	// what makes this a test of ordering rather than of the scheme check alone.
	t.Run("DangerousCallbackSchemeNotRedirected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
			Name:        testutil.GetRandomName(t),
			CallbackURL: "javascript:alert(1)",
			Scope:       sql.NullString{String: scopeInCatalog, Valid: true},
		})

		getResp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), scopeOutOfAllowlist)
		defer getResp.Body.Close()
		// 500, not 400: the request is well formed, the stored row is not. Both
		// verbs answer alike, so consolidating the two guards cannot pick a
		// status and silently regress one.
		require.Equal(t, http.StatusInternalServerError, getResp.StatusCode)
		require.Empty(t, getResp.Header.Get("Location"),
			"GET: a dangerous scheme must never reach a Location header")
		getBody := readBody(t, getResp)
		require.Contains(t, getBody, "Invalid Callback URL",
			"GET: the failure must name the callback URL, not the scope")
		require.NotContains(t, getBody, "javascript:",
			"GET: the scheme must not reach the page as a link either")

		postResp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), scopeOutOfAllowlist)
		defer postResp.Body.Close()
		require.Equal(t, http.StatusInternalServerError, postResp.StatusCode)
		require.Empty(t, postResp.Header.Get("Location"),
			"POST: a dangerous scheme must never reach a Location header")
		postBody := readBody(t, postResp)
		require.Contains(t, postBody, string(codersdk.OAuth2ErrorCodeServerError))
		// The callback-parse branch also answers server_error, so the code alone
		// does not say which guard fired.
		require.Contains(t, postBody, "invalid scheme",
			"POST: the failure must name the scheme, not just the error class")
	})

	// A registered callback may carry its own state=. Every parameter this
	// server writes onto that URL replaces what is there, so the client reads
	// back one value per parameter rather than two it may reject as malformed.
	// The cancel link, the success redirect, and the error redirect are
	// separate code paths, so all three are covered.
	t.Run("CallbackQueryParamsReplacedNotAppended", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		const presetState = "callback-preset-state"
		app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
			Name:        testutil.GetRandomName(t),
			CallbackURL: appCallbackURL + "?state=" + presetState,
			Scope:       sql.NullString{String: scopeInCatalog, Valid: true},
		})

		getResp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), "")
		defer getResp.Body.Close()
		require.Equal(t, http.StatusOK, getResp.StatusCode)
		require.NotContains(t, readBody(t, getResp), presetState,
			"the cancel link must carry the request's state, not the registered one as well")

		postResp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "")
		defer postResp.Body.Close()
		require.Equal(t, http.StatusFound, postResp.StatusCode)
		location, err := url.Parse(postResp.Header.Get("Location"))
		require.NoError(t, err)
		require.Equal(t, []string{authorizeState}, location.Query()["state"],
			"the success redirect must carry exactly one state")

		// Every other rejection test registers a callback carrying no query of
		// its own, so flipping the error redirect back to Add leaves them green.
		errResp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), scopeOutOfAllowlist)
		defer errResp.Body.Close()
		require.Equal(t, http.StatusFound, errResp.StatusCode)
		errLocation, err := url.Parse(errResp.Header.Get("Location"))
		require.NoError(t, err)
		// Pinned so the arm cannot pass on a redirect that failed ahead of the
		// error helper.
		require.Equal(t, string(codersdk.OAuth2ErrorCodeInvalidScope), errLocation.Query().Get("error"))
		require.Equal(t, []string{authorizeState}, errLocation.Query()["state"],
			"the error redirect must carry exactly one state")
	})
}

// Registration performs no catalog validation, so an app can register an
// allowlist this server cannot grant from. Authorization then rejects it rather
// than granting a scope dbauthz cannot evaluate.
func TestOAuth2AuthorizeDCRScopeCompatibility(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	oauth2providertest.EnableDCR(t, client)

	ctx := testutil.Context(t, testutil.WaitLong)
	registration, err := client.PostOAuth2ClientRegistration(ctx, codersdk.OAuth2ClientRegistrationRequest{
		RedirectURIs: []string{appCallbackURL},
		ClientName:   testutil.GetRandomName(t),
		Scope:        "openid profile email",
	})
	require.NoError(t, err, "registration performs no catalog check")

	t.Run("RequestingRegisteredScopeRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		resp := authorizeRequest(ctx, t, client, http.MethodPost, registration.ClientID, "openid")
		defer resp.Body.Close()

		requireInvalidScope(t, resp, reasonUnknownScope)
	})

	t.Run("OmittingScopeRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		resp := authorizeRequest(ctx, t, client, http.MethodPost, registration.ClientID, "")
		defer resp.Body.Close()

		requireInvalidScope(t, resp, reasonNoGrantableScope)
	})

	t.Run("RejectionNamesTheRegisteredScopes", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		resp := authorizeRequest(ctx, t, client, http.MethodGet, registration.ClientID, "")
		defer resp.Body.Close()
		requireInvalidScope(t, resp, reasonNoGrantableScope)

		location, err := url.Parse(resp.Header.Get("Location"))
		require.NoError(t, err)
		require.Contains(t, location.Query().Get("error_description"), "openid profile email",
			"the rejection must name the registered scopes the owner has to change")
	})
}

// authorizeQuery builds a well-formed /oauth2/authorize query. Callers varying
// another parameter mutate the result before sending it.
func authorizeQuery(t *testing.T, clientID, scope string) url.Values {
	t.Helper()

	_, challenge := oauth2providertest.GeneratePKCE(t)
	query := url.Values{}
	query.Set("client_id", clientID)
	query.Set("response_type", "code")
	query.Set("state", authorizeState)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	if scope != "" {
		query.Set("scope", scope)
	}
	return query
}

// authorizeRequest issues an /oauth2/authorize request without following
// redirects, so a successful POST surfaces as a 302 carrying the code.
func authorizeRequest(ctx context.Context, t *testing.T, client *codersdk.Client, method, clientID, scope string) *http.Response {
	t.Helper()

	return sendAuthorizeRequest(ctx, t, client, method, authorizeQuery(t, clientID, scope))
}

func sendAuthorizeRequest(ctx context.Context, t *testing.T, client *codersdk.Client, method string, query url.Values) *http.Response {
	t.Helper()

	authURL, err := url.Parse(client.URL.String() + "/oauth2/authorize")
	require.NoError(t, err)
	authURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, method, authURL.String(), nil)
	require.NoError(t, err)
	req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())

	httpClient := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := httpClient.Do(req)
	require.NoError(t, err)
	return resp
}

// persistedCodeScope returns the scope stored on the issued code, which is what
// the token exchange later reads.
func persistedCodeScope(ctx context.Context, t *testing.T, db database.Store, resp *http.Response) string {
	t.Helper()

	require.Equal(t, http.StatusFound, resp.StatusCode)
	location, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)

	formatted := location.Query().Get("code")
	require.NotEmpty(t, formatted, "authorization did not issue a code")

	parsed, err := oauth2provider.ParseFormattedSecret(formatted)
	require.NoError(t, err)

	code, err := db.GetOAuth2ProviderAppCodeByPrefix(ctx, []byte(parsed.Prefix))
	require.NoError(t, err)
	return code.Scope
}

// The rejection reasons from authorize.go, each unique to one branch. The wire
// carries only the rendered description, so these stand in for errors.Is.
var (
	reasonUnknownScope     = oauth2provider.ReasonUnknownScope
	reasonNoGrantableScope = oauth2provider.ReasonNoGrantableScope
	reasonScopeNotAllowed  = oauth2provider.ReasonScopeNotAllowed
)

// requireInvalidScope asserts the RFC 6749 §4.1.2.1 rejection: a redirect to
// the app's registered callback carrying the error code, a description from
// the branch the caller named, and the request's state, but no code.
func requireInvalidScope(t *testing.T, resp *http.Response, wantReason string) {
	t.Helper()

	require.Equal(t, http.StatusFound, resp.StatusCode)

	location, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	require.Equal(t, appCallbackURL, location.Scheme+"://"+location.Host+location.Path,
		"the error must go to the app's registered callback and nowhere else")

	query := location.Query()
	require.Equal(t, string(codersdk.OAuth2ErrorCodeInvalidScope), query.Get("error"))
	require.Contains(t, query.Get("error_description"), wantReason,
		"the rejection must come from the branch this case covers")
	require.Equal(t, authorizeState, query.Get("state"),
		"the client cannot correlate the failure with its request without its state")
	require.Empty(t, query.Get("code"), "a rejected request must not issue a code")
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}
