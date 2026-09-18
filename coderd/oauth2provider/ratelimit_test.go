package oauth2provider_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/oauth2provider/oauth2providertest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestOAuth2RateLimit checks that every route under /oauth2 shares the login
// rate limit. Each case uses its own server so the buckets do not overlap.
func TestOAuth2RateLimit(t *testing.T) {
	t.Parallel()

	const rateLimit = 3

	newServer := func(t *testing.T) (*codersdk.Client, codersdk.CreateFirstUserResponse, string) {
		client := coderdtest.New(t, &coderdtest.Options{
			LoginRateLimit: rateLimit,
		})
		owner := coderdtest.CreateFirstUser(t, client)
		return client, owner, client.URL.String()
	}

	// requireLimited sends the request one more time than the limit allows.
	// Every request inside the limit must get wantStatus, and the next one
	// must be refused with 429.
	requireLimited := func(t *testing.T, send func() *http.Response, wantStatus int) {
		t.Helper()
		for i := range rateLimit {
			resp := send()
			_ = resp.Body.Close()
			require.Equal(t, wantStatus, resp.StatusCode, "request %d should be inside the limit", i+1)
		}
		resp := send()
		_ = resp.Body.Close()
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "request %d should be rate limited", rateLimit+1)
	}

	t.Run("Tokens", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, _, baseURL := newServer(t)
		app, _ := oauth2providertest.CreateTestOAuth2App(t, client)

		form := url.Values{}
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", "coder_wrongprefix_wrongsecret")
		form.Set("client_id", app.ID.String())
		form.Set("client_secret", "coder_wrongprefix_wrongsecret")

		requireLimited(t, func() *http.Response {
			return doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/tokens", strings.NewReader(form.Encode()), formContentType)
		}, http.StatusUnauthorized)
	})

	t.Run("Revoke", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, _, baseURL := newServer(t)
		app, _ := oauth2providertest.CreateTestOAuth2App(t, client)

		form := url.Values{}
		form.Set("token", "coder_wrongprefix_wrongsecret")
		form.Set("client_id", app.ID.String())
		form.Set("client_secret", "coder_wrongprefix_wrongsecret")

		requireLimited(t, func() *http.Response {
			return doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/revoke", strings.NewReader(form.Encode()), formContentType)
		}, http.StatusUnauthorized)
	})

	t.Run("Register", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, _, baseURL := newServer(t)
		oauth2providertest.EnableDCR(t, client)

		body, err := json.Marshal(codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/callback"},
		})
		require.NoError(t, err)

		requireLimited(t, func() *http.Response {
			return doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/register", strings.NewReader(string(body)), jsonContentType)
		}, http.StatusCreated)
	})

	// The RFC 7592 routes carry the client ID in the path. A caller that
	// changes the ID on every request must still draw from one bucket.
	t.Run("ClientConfigurationVaryingClientID", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, _, baseURL := newServer(t)

		requireLimited(t, func() *http.Response {
			uri := baseURL + "/oauth2/clients/" + uuid.NewString()
			return doRequest(ctx, t, http.MethodGet, uri, nil, bearer("wrongtoken"))
		}, http.StatusUnauthorized)
	})

	// DELETE /oauth2/tokens is authenticated, so the limiter keys on the user
	// instead of the address and one user cannot spend another user's budget.
	t.Run("DeleteTokensLimitedPerUser", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, owner, baseURL := newServer(t)
		other, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
		uri := baseURL + "/oauth2/tokens?client_id=" + app.ID.String()

		requireLimited(t, func() *http.Response {
			return doRequest(ctx, t, http.MethodDelete, uri, nil, sessionToken(client))
		}, http.StatusNoContent)

		resp := doRequest(ctx, t, http.MethodDelete, uri, nil, sessionToken(other))
		_ = resp.Body.Close()
		require.Equal(t, http.StatusNoContent, resp.StatusCode, "a second user should have its own bucket")
	})
}
