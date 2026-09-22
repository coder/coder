package oauth2provider_test

import (
	"context"
	"encoding/json"
	"io"
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

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		var oauthErr codersdk.OAuth2Error
		require.NoError(t, json.Unmarshal(body, &oauthErr), "a refusal should carry an RFC 6749 body")
		require.Equal(t, codersdk.OAuth2ErrorCodeTemporarilyUnavailable, oauthErr.Error)
		// The dashboard and codersdk read message, not error_description.
		var apiErr codersdk.Response
		require.NoError(t, json.Unmarshal(body, &apiErr))
		require.Equal(t, oauthErr.ErrorDescription, apiErr.Message)
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

		resp := doRequest(ctx, t, http.MethodGet, authorizeURL(baseURL, uuid.NewString(), challenge), nil)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "an unknown client should be refused before the app lookup")
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

		form.Set("client_id", uuid.NewString())
		resp := doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/tokens", strings.NewReader(form.Encode()), formContentType)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "an unknown client should be refused before the app lookup")
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

		form.Set("client_id", uuid.NewString())
		resp := doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/revoke", strings.NewReader(form.Encode()), formContentType)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "an unknown client should be refused before the app lookup")
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

		type endpoint struct {
			send       func(ctx context.Context, t *testing.T, baseURL, clientID string) *http.Response
			wantStatus int
		}
		authorize := endpoint{
			send: func(ctx context.Context, t *testing.T, baseURL, clientID string) *http.Response {
				_, challenge := oauth2providertest.GeneratePKCE(t)
				return doRequest(ctx, t, http.MethodGet, authorizeURL(baseURL, clientID, challenge), nil)
			},
			wantStatus: http.StatusSeeOther,
		}
		tokens := endpoint{
			send: func(ctx context.Context, t *testing.T, baseURL, clientID string) *http.Response {
				form := url.Values{}
				form.Set("grant_type", "refresh_token")
				form.Set("refresh_token", "coder_wrongprefix_wrongsecret")
				form.Set("client_id", clientID)
				form.Set("client_secret", "coder_wrongprefix_wrongsecret")
				return doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/tokens", strings.NewReader(form.Encode()), formContentType)
			},
			wantStatus: http.StatusUnauthorized,
		}
		revoke := endpoint{
			send: func(ctx context.Context, t *testing.T, baseURL, clientID string) *http.Response {
				form := url.Values{}
				form.Set("token", "coder_wrongprefix_wrongsecret")
				form.Set("client_id", clientID)
				form.Set("client_secret", "coder_wrongprefix_wrongsecret")
				return doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/revoke", strings.NewReader(form.Encode()), formContentType)
			},
			wantStatus: http.StatusUnauthorized,
		}
		clients := endpoint{
			send: func(ctx context.Context, t *testing.T, baseURL, _ string) *http.Response {
				return doRequest(ctx, t, http.MethodGet, baseURL+"/oauth2/clients/"+uuid.NewString(), nil, bearer("wrongtoken"))
			},
			wantStatus: http.StatusUnauthorized,
		}

		for _, tc := range []struct {
			name     string
			method   string
			junkPath string
			endpoint endpoint
		}{
			{name: "OutsideAnyEndpoint", method: http.MethodGet, junkPath: "/oauth2/does-not-exist", endpoint: revoke},
			{name: "AuthorizeSuffix", method: http.MethodGet, junkPath: "/oauth2/authorize/does-not-exist", endpoint: authorize},
			{name: "TokensSuffix", method: http.MethodPost, junkPath: "/oauth2/tokens/does-not-exist", endpoint: tokens},
			{name: "RevokeSuffix", method: http.MethodPost, junkPath: "/oauth2/revoke/does-not-exist", endpoint: revoke},
			{name: "RevokeWrongMethod", method: http.MethodGet, junkPath: "/oauth2/revoke", endpoint: revoke},
			{name: "ClientsSuffix", method: http.MethodGet, junkPath: "/oauth2/clients/" + uuid.NewString() + "/does-not-exist", endpoint: clients},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctx := testutil.Context(t, testutil.WaitLong)
				client, _, baseURL := newServer(t)
				app, _ := oauth2providertest.CreateTestOAuth2App(t, client)

				for i := range rateLimit + 1 {
					resp := doRequest(ctx, t, tc.method, baseURL+tc.junkPath, nil)
					_ = resp.Body.Close()
					require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode, "junk request %d should not be counted", i+1)
				}

				requireLimited(t, func() *http.Response {
					return tc.endpoint.send(ctx, t, baseURL, app.ID.String())
				}, tc.endpoint.wantStatus)
			})
		}
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
