package oauth2provider_test

import (
	"context"
	"database/sql"
	"encoding/json"
	htmltemplate "html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
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

// Scope names used by the negotiation tests. Whether a name is in
// rbac.IsExternalScope's curated catalog is the point of each case, so the two
// groups are named rather than inlined.
const (
	scopeInCatalog     = "coder:workspaces.access"
	scopeAlsoInCatalog = "coder:templates.build"
	scopeOutOfCatalog  = "some_removed_scope"
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
			CallbackURL: "https://example.com/callback",
			Scope:       appScope,
		})
	}

	// AC1: a scope outside the app's allowlist is rejected and no code is
	// issued.
	t.Run("OutOfAllowlistRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), scopeInCatalog+" template:read")
		defer resp.Body.Close()

		requireInvalidScope(t, resp, reasonScopeNotAllowed)
	})

	// AC1's catalog half: a scope name the enforcement layer cannot evaluate is
	// rejected on its own terms, not because of the allowlist.
	t.Run("UnknownScopeRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "not_a_real_scope")
		defer resp.Body.Close()

		requireInvalidScope(t, resp, reasonUnknownScope)
	})

	// AC2: omitting scope grants the app's full allowlist (RFC 6749 §3.3).
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

		require.Equal(t, database.OAuth2ScopeUnrestricted, persistedCodeScope(ctx, t, db, resp))
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

	// AC3: apps with no configured allowlist keep today's unrestricted
	// behavior. The persisted value is asserted literally, since '' is what the
	// column's CHECK would reject.
	t.Run("NoAllowlistStaysUnrestricted", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "")
		defer resp.Body.Close()

		require.Equal(t, database.OAuth2ScopeUnrestricted, persistedCodeScope(ctx, t, db, resp))
	})

	// AC16: NULL (admin-created apps) and '' (DCR apps that sent no scope) are
	// one "no allowlist configured" state and must behave identically.
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
		require.Equal(t, database.OAuth2ScopeUnrestricted, nullScope)
		require.Equal(t, nullScope, emptyScope)
	})

	// Edge Case 20: an allowlist entry no longer in the catalog is dropped, not
	// granted. Paired with AllowlistFilteringToEmptyRejected below, which is the
	// same filter with no survivors.
	t.Run("StaleAllowlistEntryDropped", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog + " " + scopeOutOfCatalog, Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "")
		defer resp.Body.Close()

		require.Equal(t, scopeInCatalog, persistedCodeScope(ctx, t, db, resp))
	})

	// AC15: an allowlist whose every entry is dropped rejects rather than
	// falling back to unrestricted, which would grant strictly more than the
	// allowlist ever permitted.
	t.Run("AllowlistFilteringToEmptyRejected", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: "openid profile email", Valid: true})
		resp := authorizeRequest(ctx, t, client, http.MethodPost, app.ID.String(), "")
		defer resp.Body.Close()

		requireInvalidScope(t, resp, reasonNoGrantableScope)
	})

	// AC8: the GET handler rejects before the consent page renders, so the user
	// is never asked to approve a request that cannot succeed.
	t.Run("ConsentPageNotRenderedForInvalidScope", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app := seedApp(t, sql.NullString{String: scopeInCatalog, Valid: true})

		resp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), scopeInCatalog+" template:read")
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.NotContains(t, readBody(t, resp), `id="allow-form"`,
			"the consent page must not render for a scope the app cannot be granted")

		// Positive control: the same handler still renders the consent page for
		// a request the app can be granted, so the assertion above is about the
		// scope and not about the request shape.
		okResp := authorizeRequest(ctx, t, client, http.MethodGet, app.ID.String(), scopeInCatalog)
		defer okResp.Body.Close()
		require.Equal(t, http.StatusOK, okResp.StatusCode)
		require.Contains(t, readBody(t, okResp), `id="allow-form"`)
	})
}

// TestOAuth2AuthorizeDCRScopeCompatibility pins the compatibility break
// §4.2.2 accepts: dynamic client registration performs no catalog validation,
// so an app can register an allowlist this server cannot grant from. Both
// directions fail, and both fail loudly with invalid_scope rather than
// silently granting a scope dbauthz has no way to evaluate.
func TestOAuth2AuthorizeDCRScopeCompatibility(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	oauth2providertest.EnableDCR(t, client)

	ctx := testutil.Context(t, testutil.WaitLong)
	registration, err := client.PostOAuth2ClientRegistration(ctx, codersdk.OAuth2ClientRegistrationRequest{
		RedirectURIs: []string{"https://example.com/callback"},
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
}

// authorizeRequest issues an /oauth2/authorize request for the given app.
// Redirects are not followed, so a successful POST surfaces as a 302 whose
// Location carries the code.
func authorizeRequest(ctx context.Context, t *testing.T, client *codersdk.Client, method, clientID, scope string) *http.Response {
	t.Helper()

	authURL, err := url.Parse(client.URL.String() + "/oauth2/authorize")
	require.NoError(t, err)

	_, challenge := oauth2providertest.GeneratePKCE(t)
	query := url.Values{}
	query.Set("client_id", clientID)
	query.Set("response_type", "code")
	query.Set("state", uuid.NewString())
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	if scope != "" {
		query.Set("scope", scope)
	}
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
	reasonNoGrantableScope = "contains no grantable scope"
	reasonScopeNotAllowed  = "not in this app's allowed scope list"
)

// requireInvalidScope asserts the RFC 6749 §4.1.2.1 rejection, that it came
// from the branch the caller named, and that the request produced no
// authorization code at all.
func requireInvalidScope(t *testing.T, resp *http.Response, wantReason string) {
	t.Helper()

	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Empty(t, resp.Header.Get("Location"), "a rejected request must not redirect with a code")

	var errResp oauth2providertest.OAuth2Error
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&errResp))
	require.Equal(t, string(codersdk.OAuth2ErrorCodeInvalidScope), errResp.Error)
	require.Contains(t, errResp.ErrorDescription, wantReason,
		"the rejection must come from the branch this case covers")
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}
