package oauth2provider_test

import (
	"context"
	"database/sql"
	"html"
	htmltemplate "html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
		// The submit and cancel handlers hide the list by this id.
		assert.Contains(t, body, `id="scope-list"`)
		assert.Contains(t, body, `id="scope-disclaimer"`)
		assert.Contains(t, body, `id="allow-form"`)
		assert.Contains(t, body, `id="cancel-link"`)
	})

	t.Run("UnrestrictedStaysFullAccess", func(t *testing.T) {
		t.Parallel()

		body := render(t, nil, true)
		assert.Contains(t, body, "full access")
		assert.NotContains(t, body, `id="scope-list"`)
		assert.NotContains(t, body, `id="scope-disclaimer"`)
	})

	// Unreachable today; the guard is for a future caller computing the grant
	// itself. An empty grant is the opposite of an unrestricted one, so falling
	// back to the length of Scopes would describe it as full access.
	t.Run("EmptyScopesAreRefused", func(t *testing.T) {
		t.Parallel()

		rec := record(t, []string{}, false)
		require.Equal(t, http.StatusInternalServerError, rec.Result().StatusCode)
		body := rec.Body.String()
		assert.NotContains(t, body, "full access")
		assert.NotContains(t, body, `id="allow-form"`)
		assert.NotContains(t, body, `id="scope-list"`)
	})

	// A guard written against one spelling would let the other through.
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

	t.Run("ConsentPageNotRenderedForInvalidScope", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})

		resp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), scopeInCatalog+" "+scopeOutOfAllowlist)
		defer resp.Body.Close()
		requireInvalidScope(t, resp, reasonScopeNotAllowed)
	})

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
	// empty state is not the same as no state.
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
		// So the case cannot pass on a redirect that failed earlier.
		require.Equal(t, string(codersdk.OAuth2ErrorCodeInvalidScope), location.Query().Get("error"))
		require.False(t, location.Query().Has("state"),
			"a client that sent no state must not receive an empty one")
	})

	// redirect_uri validation running first is what keeps the rejection
	// redirect above from being reachable with a request-supplied URI.
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
			// The request also carries an invalid scope, so this pins which
			// guard rejected it first.
			require.Contains(t, readBody(t, resp), "must exactly match",
				"%s: the rejection must come from redirect_uri validation", method)
		}

		// Positive control: the same handler still renders the consent page.
		okResp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), scopeInCatalog)
		defer okResp.Body.Close()
		require.Equal(t, http.StatusOK, okResp.StatusCode)
		require.Contains(t, readBody(t, okResp), `id="allow-form"`)
	})

	// The request also carries a scope the app cannot be granted, so the
	// rejection redirect is the write that would otherwise fire. This is a test
	// of ordering, not of the scheme check alone.
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
		// 500, not 400: the request is well formed, the stored row is not.
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
		// The callback-parse branch also answers server_error.
		require.Contains(t, postBody, "invalid scheme",
			"POST: the failure must name the scheme, not just the error class")
	})

	// A registered callback may carry its own state=, and the cancel link, the
	// success redirect, and the error redirect write onto it separately.
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

		errResp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), scopeOutOfAllowlist)
		defer errResp.Body.Close()
		require.Equal(t, http.StatusFound, errResp.StatusCode)
		errLocation, err := url.Parse(errResp.Header.Get("Location"))
		require.NoError(t, err)
		// So the arm cannot pass on a redirect that failed earlier.
		require.Equal(t, string(codersdk.OAuth2ErrorCodeInvalidScope), errLocation.Query().Get("error"))
		require.Equal(t, []string{authorizeState}, errLocation.Query()["state"],
			"the error redirect must carry exactly one state")
	})

	// Registration validates the scheme and rejects fragments, so a callback
	// registered with error= or code= in its query is accepted and reaches
	// every response built from it.
	t.Run("RegisteredResponseParamsDroppedRestRetained", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
			Name:        testutil.GetRandomName(t),
			CallbackURL: appCallbackURL + "?tenant=acme&error=preset&code=preset",
			Scope:       sql.NullString{String: scopeInCatalog, Valid: true},
		})

		postResp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "")
		defer postResp.Body.Close()
		require.Equal(t, http.StatusFound, postResp.StatusCode)
		location, err := url.Parse(postResp.Header.Get("Location"))
		require.NoError(t, err)
		success := location.Query()
		require.Empty(t, success.Get("error"),
			"a client reading error first would discard the code this response carries")
		require.NotEqual(t, "preset", success.Get("code"),
			"the code must be the one just issued, not the registered value")
		require.NotEmpty(t, success.Get("code"))
		require.Equal(t, "acme", success.Get("tenant"),
			"§3.1.2 requires retaining the rest of the registered query")

		errResp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), scopeOutOfAllowlist)
		defer errResp.Body.Close()
		require.Equal(t, http.StatusFound, errResp.StatusCode)
		errLocation, err := url.Parse(errResp.Header.Get("Location"))
		require.NoError(t, err)
		failure := errLocation.Query()
		require.Equal(t, string(codersdk.OAuth2ErrorCodeInvalidScope), failure.Get("error"))
		require.Empty(t, failure.Get("code"),
			"a rejected request must not appear to carry a code")
		require.Equal(t, "acme", failure.Get("tenant"))
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

func TestOAuth2AuthorizeErrorsReachTheClient(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	_ = coderdtest.CreateFirstUser(t, client)

	seedApp := func(t *testing.T) database.OAuth2ProviderApp {
		t.Helper()
		return dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
			Name:        testutil.GetRandomName(t),
			CallbackURL: appCallbackURL,
			Scope:       sql.NullString{String: scopeInCatalog, Valid: true},
		})
	}

	t.Run("UnsupportedResponseTypeRedirected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t)

		for _, method := range []string{http.MethodGet, http.MethodPost} {
			query := authorizeQuery(t, app.ID.String(), scopeInCatalog)
			query.Set("response_type", "token")

			resp := sendAuthorizeRequest(ctx, t, client, method, query)
			defer resp.Body.Close()

			requireAuthorizeErrorRedirect(t, resp,
				codersdk.OAuth2ErrorCodeUnsupportedResponseType,
				"Only response_type=code is supported")
		}
	})

	t.Run("UnparseableResponseTypeNotRedirected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t)

		for _, method := range []string{http.MethodGet, http.MethodPost} {
			query := authorizeQuery(t, app.ID.String(), scopeInCatalog)
			query.Set("response_type", "not_a_response_type")

			resp := sendAuthorizeRequest(ctx, t, client, method, query)
			defer resp.Body.Close()

			require.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"%s: extractAuthorizeParams failures answer on Coder whether or not the callback was trustworthy, and this request omits redirect_uri, so it was", method)
			require.Empty(t, resp.Header.Get("Location"),
				"%s: nothing may be redirected from inside extractAuthorizeParams", method)
		}
	})

	// GET as well as POST, since the consent page must not render for a method
	// the POST will refuse after the user clicks Allow. Explicit as well as
	// omitted redirect_uri, since the two take different paths through the
	// parser and must reach the same check.
	t.Run("InvalidPKCEMethodRedirected", func(t *testing.T) {
		t.Parallel()

		app := seedApp(t)
		for _, tc := range []struct {
			name        string
			method      string
			redirectURI string
		}{
			{"GET", http.MethodGet, ""},
			{"GETExplicitRedirectURI", http.MethodGet, appCallbackURL},
			{"POST", http.MethodPost, ""},
			{"POSTExplicitRedirectURI", http.MethodPost, appCallbackURL},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)

				query := authorizeQuery(t, app.ID.String(), scopeInCatalog)
				query.Set("code_challenge_method", "plain")
				if tc.redirectURI != "" {
					query.Set("redirect_uri", tc.redirectURI)
				}

				resp := sendAuthorizeRequest(ctx, t, client, tc.method, query)
				defer resp.Body.Close()

				requireAuthorizeErrorRedirect(t, resp,
					codersdk.OAuth2ErrorCodeInvalidRequest, "use 'S256'")
			})
		}
	})

	t.Run("CancelLinkCarriesAccessDenied", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t)
		resp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), scopeInCatalog)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		body := readBody(t, resp)

		cancel := cancelLinkFromConsentPage(t, body)
		require.Equal(t, appCallbackURL, cancel.Scheme+"://"+cancel.Host+cancel.Path,
			"canceling must return the user to the app's registered callback")
		query := cancel.Query()
		require.Equal(t, string(codersdk.OAuth2ErrorCodeAccessDenied), query.Get("error"))
		require.Equal(t, authorizeState, query.Get("state"))
		require.Empty(t, query.Get("code"), "declining must not issue a code")
	})
}

// The consent page is a Go template with no test seam, so the href has to be
// read back out of the rendered HTML.
func cancelLinkFromConsentPage(t *testing.T, body string) *url.URL {
	t.Helper()

	const marker = `id="cancel-link" href="`
	start := strings.Index(body, marker)
	require.GreaterOrEqual(t, start, 0, "consent page has no cancel link")
	rest := body[start+len(marker):]
	end := strings.Index(rest, `"`)
	require.GreaterOrEqual(t, end, 0, "cancel link href is unterminated")

	cancel, err := url.Parse(html.UnescapeString(rest[:end]))
	require.NoError(t, err)
	return cancel
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

func requireAuthorizeErrorRedirect(t *testing.T, resp *http.Response, wantCode codersdk.OAuth2ErrorCode, wantDescription string) {
	t.Helper()

	require.Equal(t, http.StatusFound, resp.StatusCode)

	location, err := url.Parse(resp.Header.Get("Location"))
	require.NoError(t, err)
	require.Equal(t, appCallbackURL, location.Scheme+"://"+location.Host+location.Path,
		"the error must go to the app's registered callback and nowhere else")

	query := location.Query()
	require.Equal(t, string(wantCode), query.Get("error"))
	require.Contains(t, query.Get("error_description"), wantDescription,
		"the rejection must come from the branch this case covers")
	// Every §4.1.2.1 description is built at one chokepoint, so asserting the
	// charset here covers each branch that reaches this helper.
	for _, r := range query.Get("error_description") {
		require.True(t, r == 0x20 || r == 0x21 || (r >= 0x23 && r <= 0x5B) || (r >= 0x5D && r <= 0x7E),
			"error_description carries %q, outside the NQSCHAR set RFC 6749 Appendix A permits", r)
	}
	require.Equal(t, authorizeState, query.Get("state"),
		"the client cannot correlate the failure with its request without its state")
	require.Empty(t, query.Get("code"), "a rejected request must not issue a code")
}

func requireInvalidScope(t *testing.T, resp *http.Response, wantReason string) {
	t.Helper()

	requireAuthorizeErrorRedirect(t, resp, codersdk.OAuth2ErrorCodeInvalidScope, wantReason)
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}
