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
	})

	require.Equal(t, http.StatusOK, rec.Result().StatusCode)
	body := rec.Body.String()
	assert.Contains(t, body, `name="csrf_token"`)
	assert.Contains(t, body, `value="`+csrfFieldValue+`"`)
	assert.Contains(t, body, `id="allow-form"`)
	assert.Contains(t, body, `id="cancel-link"`)
}

// The consent page is the only place a person is told what they are about to
// approve, so what it states has to follow the negotiated scope rather than a
// fixed sentence. Both directions are asserted: a narrow grant must not be
// described as full access, and a full grant must not be described by a scope
// name no user would recognize.
func TestOAuthConsentFormStatesNegotiatedScope(t *testing.T) {
	t.Parallel()

	render := func(t *testing.T, scopes []string) string {
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
		})
		require.Equal(t, http.StatusOK, rec.Result().StatusCode)
		return rec.Body.String()
	}

	t.Run("NarrowScopeListed", func(t *testing.T) {
		t.Parallel()

		body := render(t, []string{"workspace:ssh", "template:read"})
		assert.Contains(t, body, "workspace:ssh")
		assert.Contains(t, body, "template:read")
		assert.NotContains(t, body, "full access",
			"a scoped grant must not be described as full access")
		// The approval controls must survive the added branch, since a page
		// that states the scope but cannot be submitted is worse than the
		// fixed sentence it replaced.
		assert.Contains(t, body, `id="allow-form"`)
		assert.Contains(t, body, `id="cancel-link"`)
	})

	t.Run("UnrestrictedStaysFullAccess", func(t *testing.T) {
		t.Parallel()

		body := render(t, nil)
		assert.Contains(t, body, "full access")
		assert.NotContains(t, body, `id="scope-list"`)
	})
}

// Scope names used by the negotiation tests. Whether a name is in
// rbac.IsExternalScope's curated catalog is the point of each case, so the two
// groups are named rather than inlined.
const (
	scopeInCatalog     = "coder:workspaces.access"
	scopeAlsoInCatalog = "coder:templates.build"
	scopeOutOfCatalog  = "some_removed_scope"
	// In the catalog, and outside the authority scopeInCatalog carries: that
	// composite grants template:read but never template:update.
	scopeOutOfAllowlist = "template:update"
)

// The callback every app in these tests registers, and the state every request
// sends. A rejection redirects to the first carrying the second, so both are
// named rather than inlined.
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

	// Each sub-test gets its own app: only one code exists per app/user pair at
	// a time, and the allowlist is the variable under test.
	seedApp := func(t *testing.T, appScope sql.NullString) database.OAuth2ProviderApp {
		t.Helper()
		return dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
			Name:        testutil.GetRandomName(t),
			CallbackURL: appCallbackURL,
			Scope:       appScope,
		})
	}

	// A scope outside the app's allowlist is rejected and no code is issued.
	t.Run("OutOfAllowlistRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), scopeInCatalog+" "+scopeOutOfAllowlist)
		defer resp.Body.Close()

		requireInvalidScope(t, resp, reasonScopeNotAllowed)
	})

	// The allowlist bounds authority rather than spelling, so a name it never
	// lists is still granted when the permissions it expands to are ones the
	// allowlist already carries.
	t.Run("ScopeCoveredByAllowlistGranted", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "workspace:ssh")
		defer resp.Body.Close()

		require.Equal(t, "workspace:ssh", persistedCodeScope(ctx, t, db, resp))
	})

	// The catalog half of the same guarantee: a scope name the enforcement
	// layer cannot evaluate is rejected on its own terms, not because of the
	// allowlist.
	t.Run("UnknownScopeRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "not_a_real_scope")
		defer resp.Body.Close()

		requireInvalidScope(t, resp, reasonUnknownScope)
	})

	// Omitting scope grants the app's full allowlist (RFC 6749 §3.3).
	t.Run("OmittedScopeDefaultsToAllowlist", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		allowlist := scopeInCatalog + " " + scopeAlsoInCatalog
		app := seedApp(t, sql.NullString{String: allowlist, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "")
		defer resp.Body.Close()

		require.Equal(t, allowlist, persistedCodeScope(ctx, t, db, resp))
	})

	t.Run("RequestedSubsetGranted", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog + " " + scopeAlsoInCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), scopeAlsoInCatalog)
		defer resp.Body.Close()

		require.Equal(t, scopeAlsoInCatalog, persistedCodeScope(ctx, t, db, resp))
	})

	// rbac.IsExternalScope accepts `all` as a backward-compatible alias, but
	// the api_key_scope enum has only `coder:all`. Asserted against the stored
	// row rather than the negotiation's return value, because the column's
	// vocabulary is what the claim is about.
	t.Run("LegacyAliasPersistedCanonically", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "all")
		defer resp.Body.Close()

		require.Equal(t, string(database.ApiKeyScopeCoderAll), persistedCodeScope(ctx, t, db, resp))
	})

	// A repeated scope denotes one grant, so it is stored once.
	t.Run("DuplicateRequestedScopePersistedOnce", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), scopeInCatalog+" "+scopeInCatalog)
		defer resp.Body.Close()

		require.Equal(t, scopeInCatalog, persistedCodeScope(ctx, t, db, resp))
	})

	// Apps with no configured allowlist keep today's unrestricted behavior. The persisted value is asserted literally, since '' is what the
	// column's CHECK would reject.
	t.Run("NoAllowlistStaysUnrestricted", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "")
		defer resp.Body.Close()

		require.Equal(t, string(database.ApiKeyScopeCoderAll), persistedCodeScope(ctx, t, db, resp))
	})

	// NULL (admin-created apps) and '' (DCR apps that sent no scope) are one
	// "no allowlist configured" state and must behave identically.
	t.Run("NullAndEmptyAllowlistBehaveIdentically", func(t *testing.T) {
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

	// An allowlist entry no longer in the catalog is dropped, not granted. Paired with AllowlistFilteringToEmptyRejected below, which is the
	// same filter with no survivors.
	t.Run("StaleAllowlistEntryDropped", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog + " " + scopeOutOfCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "")
		defer resp.Body.Close()

		require.Equal(t, scopeInCatalog, persistedCodeScope(ctx, t, db, resp))
	})

	// An allowlist whose every entry is dropped rejects rather than falling
	// back to unrestricted, which would grant strictly more than the
	// allowlist ever permitted.
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
		require.NotContains(t, readBody(t, resp), `id="allow-form"`,
			"the consent page must not render for a scope the app cannot be granted")
	})

	// The wiring rather than the template: the page a user is actually served
	// must name the scope the code will carry. Its rejection counterpart is
	// ConsentPageNotRenderedForInvalidScope above.
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
		// The page must state the grant, not the ceiling it was drawn from.
		// The allowlist here covers workspace:ssh and more, so showing the
		// allowlist would still satisfy every assertion above while telling
		// the user they are approving more than the code will carry.
		require.NotContains(t, body, scopeInCatalog,
			"the consent page must state the negotiated scope, not the app's allowlist")
	})

	// The other half of RFC 6749 §4.1.2.1: a redirect URI that does not match
	// the app's registration is never a destination this server sends anyone
	// to, however the request fails. That validation running first is what
	// keeps the rejection redirect above from being reachable with a
	// request-supplied URI.
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
			// Pinned so the case cannot pass on some unrelated 400: the
			// request also carries an invalid scope, and the redirect URI is
			// what must reject it first.
			require.Contains(t, readBody(t, resp), "must exactly match",
				"%s: the rejection must come from redirect_uri validation", method)
		}

		// Positive control: the same handler still renders the consent page for
		// a request the app can be granted, so the assertion above is about the
		// scope and not about the request shape.
		okResp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), scopeInCatalog)
		defer okResp.Body.Close()
		require.Equal(t, http.StatusOK, okResp.StatusCode)
		require.Contains(t, readBody(t, okResp), `id="allow-form"`)
	})
}

// TestOAuth2AuthorizeDCRScopeCompatibility pins an accepted compatibility
// break: dynamic client registration performs no catalog validation, so an
// app can register an allowlist this server cannot grant from. Both
// directions fail, and both fail loudly with invalid_scope rather than
// silently granting a scope dbauthz has no way to evaluate.
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
	require.NoError(t, err, "registration itself is unchanged: no catalog check happens here")

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

	// The break is only recoverable by whoever registered the app, and the
	// redirect is what reaches them: their own callback handler logs the
	// description. It has to name the scopes they registered, since the
	// request that triggered this carried none.
	t.Run("RejectionNamesTheRegisteredScopes", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		resp := authorizeRequest(ctx, t, client, http.MethodGet, registration.ClientID, "")
		defer resp.Body.Close()
		requireInvalidScope(t, resp, reasonNoGrantableScope)

		location, err := url.Parse(resp.Header.Get("Location"))
		require.NoError(t, err)
		require.Contains(t, location.Query().Get("error_description"), "openid profile email",
			"the app owner cannot act on this without knowing which registered scopes are the problem")
	})
}

// authorizeQuery builds a well-formed /oauth2/authorize query. Callers that
// need to vary a parameter the happy path does not, such as redirect_uri,
// mutate the result and pass it to sendAuthorizeRequest.
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

// authorizeRequest issues an /oauth2/authorize request for the given app.
// Redirects are not followed, so a successful POST surfaces as a 302 whose
// Location carries the code.
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

// persistedCodeScope follows a successful authorization to the code it issued
// and returns the scope recorded on that row, which is what the token exchange
// will later read.
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

// Fragments of the rejection reasons in authorize.go, each unique to one
// branch. The transport carries only the rendered description, so these pin
// over the wire what errors.Is pins in the package's own tests.
const (
	reasonUnknownScope     = "unknown or unsupported scope"
	reasonNoGrantableScope = "none of the scopes registered for this app are supported"
	reasonScopeNotAllowed  = "not in this app's allowed scope list"
)

// requireInvalidScope asserts the RFC 6749 §4.1.2.1 rejection: the client
// learns of the failure by a redirect to its own registered callback, carrying
// the error code, a description from the branch the caller named, and the
// state it sent, and carrying no authorization code.
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
