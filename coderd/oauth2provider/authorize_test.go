package oauth2provider_test

import (
	"context"
	htmltemplate "html/template"
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

const (
	appCallbackURL = "https://example.com/callback"
	authorizeState = "test-authorize-state"
)

func TestOAuth2AuthorizeSessionCeiling(t *testing.T) {
	t.Parallel()

	db, pubsub := dbtestutil.NewDB(t)
	client := coderdtest.New(t, &coderdtest.Options{
		Database: db,
		Pubsub:   pubsub,
	})
	_ = coderdtest.CreateFirstUser(t, client)
	app := dbgen.OAuth2ProviderApp(t, db, database.OAuth2ProviderApp{
		Name:        testutil.GetRandomName(t),
		CallbackURL: appCallbackURL,
	})

	sessions := map[string]struct {
		req    codersdk.CreateTokenRequest
		reason string
	}{
		"ScopedSessionRefused": {
			req:    codersdk.CreateTokenRequest{Scopes: []codersdk.APIKeyScope{codersdk.APIKeyScopeCoderWorkspacesAccess}},
			reason: reasonBeyondSession,
		},
		"RestrictedAllowListRefused": {
			req: codersdk.CreateTokenRequest{AllowList: []codersdk.APIAllowListTarget{
				codersdk.AllowTypeTarget(codersdk.ResourceOauth2AppCodeToken),
				codersdk.AllowResourceTarget(codersdk.ResourceWorkspace, uuid.New()),
			}},
			reason: reasonSessionAllowList,
		},
	}
	for name, tc := range sessions {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			token, err := client.CreateToken(ctx, codersdk.Me, tc.req)
			require.NoError(t, err)
			session := codersdk.New(client.URL, codersdk.WithSessionToken(token.Key))
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				resp := authorizeRequest(ctx, t, session, method, app.ID.String(), "")
				requireInvalidScope(t, resp, tc.reason)
				resp.Body.Close()
			}
		})
	}
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

var (
	reasonBeyondSession    = oauth2provider.ReasonBeyondSession
	reasonSessionAllowList = oauth2provider.ReasonSessionAllowList
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
