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

	requireLimited := func(t *testing.T, send func() *http.Response, wantStatus int) {
		t.Helper()
		for i := range rateLimit {
			resp := send()
			_ = resp.Body.Close()
			require.Equal(t, wantStatus, resp.StatusCode, "request %d should be inside the limit", i+1)
		}
		resp := send()
		defer resp.Body.Close()
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "request %d should be rate limited", rateLimit+1)

		var oauthErr codersdk.OAuth2Error
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&oauthErr), "a refusal should carry an RFC 6749 body")
		require.Equal(t, codersdk.OAuth2ErrorCodeTemporarilyUnavailable, oauthErr.Error)
	}

	t.Run("Authorize", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, _, baseURL := newServer(t)
		app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
		_, challenge := oauth2providertest.GeneratePKCE(t)
		uri := authorizeURL(baseURL, app.ID.String(), challenge)

		requireLimited(t, func() *http.Response {
			return doRequest(ctx, t, http.MethodGet, uri, nil)
		}, http.StatusSeeOther)
	})

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

	t.Run("ClientConfigurationVaryingClientID", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, _, baseURL := newServer(t)

		requireLimited(t, func() *http.Response {
			uri := baseURL + "/oauth2/clients/" + uuid.NewString()
			return doRequest(ctx, t, http.MethodGet, uri, nil, bearer("wrongtoken"))
		}, http.StatusUnauthorized)
	})

	t.Run("AuthorizationServerMetadata", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, _, baseURL := newServer(t)

		requireLimited(t, func() *http.Response {
			return doRequest(ctx, t, http.MethodGet, baseURL+"/.well-known/oauth-authorization-server", nil)
		}, http.StatusOK)
	})

	t.Run("ProtectedResourceMetadata", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, _, baseURL := newServer(t)

		requireLimited(t, func() *http.Response {
			return doRequest(ctx, t, http.MethodGet, baseURL+"/.well-known/oauth-protected-resource", nil)
		}, http.StatusOK)
	})

	t.Run("ProtectedResourceMetadataVaryingSuffix", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		_, _, baseURL := newServer(t)

		requireLimited(t, func() *http.Response {
			uri := baseURL + "/.well-known/oauth-protected-resource/" + uuid.NewString()
			return doRequest(ctx, t, http.MethodGet, uri, nil)
		}, http.StatusOK)
	})

	t.Run("SeparateBucketPerEndpoint", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, _, baseURL := newServer(t)
		app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
		_, challenge := oauth2providertest.GeneratePKCE(t)
		authorize := authorizeURL(baseURL, app.ID.String(), challenge)

		for i := range rateLimit + 1 {
			resp := doRequest(ctx, t, http.MethodGet, authorize, nil)
			_ = resp.Body.Close()
			if i == rateLimit {
				require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "authorize should run out of budget")
			}
		}

		form := url.Values{}
		form.Set("token", "coder_wrongprefix_wrongsecret")
		form.Set("client_id", app.ID.String())
		form.Set("client_secret", "coder_wrongprefix_wrongsecret")

		resp := doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/revoke", strings.NewReader(form.Encode()), formContentType)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "revoke should have its own budget")
	})

	t.Run("UnmatchedPathKeepsBudget", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		client, _, baseURL := newServer(t)
		app, _ := oauth2providertest.CreateTestOAuth2App(t, client)

		for i := range rateLimit + 1 {
			resp := doRequest(ctx, t, http.MethodGet, baseURL+"/oauth2/does-not-exist", nil)
			_ = resp.Body.Close()
			require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode, "junk request %d should not be counted", i+1)
		}

		form := url.Values{}
		form.Set("token", "coder_wrongprefix_wrongsecret")
		form.Set("client_id", app.ID.String())
		form.Set("client_secret", "coder_wrongprefix_wrongsecret")

		requireLimited(t, func() *http.Response {
			return doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/revoke", strings.NewReader(form.Encode()), formContentType)
		}, http.StatusUnauthorized)
	})

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
