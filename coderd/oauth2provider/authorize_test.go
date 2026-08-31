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
		body := requireInvalidScope(t, resp, reasonNoGrantableScope)

		require.Contains(t, body, "openid profile email",
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

// requireInvalidScope asserts the request was refused by the named branch
// rather than issued a code. GET answers with an error page and POST with an
// OAuth2 error body, so the returned body is unescaped for either.
func requireInvalidScope(t *testing.T, resp *http.Response, wantReason string) string {
	t.Helper()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Empty(t, resp.Header.Get("Location"),
		"a rejected request must not be redirected, least of all with a code")

	body := html.UnescapeString(readBody(t, resp))
	require.Contains(t, body, wantReason,
		"the rejection must come from the branch this case covers")
	return body
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}
