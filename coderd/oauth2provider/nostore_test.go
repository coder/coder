package oauth2provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/oauth2provider/oauth2providertest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// TestOAuth2NoStoreHeaders asserts Cache-Control: no-store and Pragma:
// no-cache on every response from the /oauth2 tree, credential-bearing or not.
// Three write paths never call httpapi.Write, so these are what prove the
// middleware reaches them: POST /oauth2/revoke and DELETE
// /oauth2/clients/{client_id} write a bare status, and
// writeOAuth2RegistrationError encodes its own JSON.
//
// The cases at the end pin the exclusion of the /.well-known/* metadata
// endpoints, failing if the middleware is hoisted onto a higher route tree.
func TestOAuth2NoStoreHeaders(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	oauth2providertest.EnableDCR(t, client)
	baseURL := client.URL.String()

	t.Run("TokenExchange", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app, secret := oauth2providertest.CreateTestOAuth2App(t, client)
		verifier, challenge := oauth2providertest.GeneratePKCE(t)
		code := authorizationCode(t, client, baseURL, app.ID.String(), challenge)

		form := url.Values{}
		form.Set("grant_type", "authorization_code")
		form.Set("code", code)
		form.Set("client_id", app.ID.String())
		form.Set("client_secret", secret)
		form.Set("code_verifier", verifier)
		form.Set("redirect_uri", oauth2providertest.TestRedirectURI)

		resp := doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/tokens", strings.NewReader(form.Encode()), formContentType)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		requireNoStore(t, resp)
	})

	t.Run("TokenExchangeInvalidClient", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
		verifier, challenge := oauth2providertest.GeneratePKCE(t)
		code := authorizationCode(t, client, baseURL, app.ID.String(), challenge)

		form := url.Values{}
		form.Set("grant_type", "authorization_code")
		form.Set("code", code)
		form.Set("client_id", app.ID.String())
		form.Set("client_secret", "not-the-client-secret")
		form.Set("code_verifier", verifier)
		form.Set("redirect_uri", oauth2providertest.TestRedirectURI)

		resp := doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/tokens", strings.NewReader(form.Encode()), formContentType)
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		requireNoStore(t, resp)
		// Both writers coexist: WriteOAuth2Error sets this one after the
		// middleware has set its own.
		require.Equal(t, `Basic realm="coder"`, resp.Header.Get("WWW-Authenticate"))
	})

	t.Run("AuthorizeRedirect", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
		_, challenge := oauth2providertest.GeneratePKCE(t)

		resp := doRequest(ctx, t, http.MethodPost, authorizeURL(baseURL, app.ID.String(), challenge), nil, sessionToken(client))
		defer resp.Body.Close()
		require.Equal(t, http.StatusFound, resp.StatusCode)
		requireNoStore(t, resp)

		// The credential this response carries is in the Location query.
		location, err := url.Parse(resp.Header.Get("Location"))
		require.NoError(t, err)
		require.NotEmpty(t, location.Query().Get("code"))
	})

	t.Run("AuthorizeConsentPage", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
		_, challenge := oauth2providertest.GeneratePKCE(t)

		resp := doRequest(ctx, t, http.MethodGet, authorizeURL(baseURL, app.ID.String(), challenge), nil, sessionToken(client))
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		requireNoStore(t, resp)
	})

	t.Run("AuthorizeStaticErrorPage", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app, _ := oauth2providertest.CreateTestOAuth2App(t, client)
		_, challenge := oauth2providertest.GeneratePKCE(t)

		// An unsupported response_type renders a static error page rather
		// than going through httpapi.
		uri := strings.Replace(authorizeURL(baseURL, app.ID.String(), challenge), "response_type=code", "response_type=token", 1)
		resp := doRequest(ctx, t, http.MethodGet, uri, nil, sessionToken(client))
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		requireNoStore(t, resp)
	})

	t.Run("Revoke", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app, secret := oauth2providertest.CreateTestOAuth2App(t, client)
		verifier, challenge := oauth2providertest.GeneratePKCE(t)
		code := authorizationCode(t, client, baseURL, app.ID.String(), challenge)
		token := oauth2providertest.ExchangeCodeForToken(t, baseURL, oauth2providertest.TokenExchangeParams{
			GrantType:    "authorization_code",
			Code:         code,
			ClientID:     app.ID.String(),
			ClientSecret: secret,
			CodeVerifier: verifier,
			RedirectURI:  oauth2providertest.TestRedirectURI,
		})

		form := url.Values{}
		form.Set("token", token.RefreshToken)
		form.Set("client_id", app.ID.String())

		// RFC 7009 success is a bare WriteHeader(200), never httpapi.Write.
		resp := doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/revoke", strings.NewReader(form.Encode()), formContentType)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		requireNoStore(t, resp)
	})

	t.Run("DeleteTokens", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app, _ := oauth2providertest.CreateTestOAuth2App(t, client)

		uri := fmt.Sprintf("%s/oauth2/tokens?client_id=%s", baseURL, app.ID.String())
		resp := doRequest(ctx, t, http.MethodDelete, uri, nil, sessionToken(client))
		defer resp.Body.Close()
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		requireNoStore(t, resp)
	})

	t.Run("Register", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		body := registrationBody(t, codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/callback"},
			ClientName:   fmt.Sprintf("nostore-register-%s", testutil.MustRandString(t, 10)),
		})

		resp := doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/register", strings.NewReader(body), jsonContentType)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		requireNoStore(t, resp)
	})

	t.Run("RegisterInvalidMetadata", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		body := registrationBody(t, codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"not-a-url"},
		})

		// Rejected by writeOAuth2RegistrationError, which encodes its own
		// JSON rather than calling httpapi.Write.
		resp := doRequest(ctx, t, http.MethodPost, baseURL+"/oauth2/register", strings.NewReader(body), jsonContentType)
		defer resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		requireNoStore(t, resp)
	})

	t.Run("GetClientConfiguration", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		registration := registerClient(ctx, t, client)

		uri := fmt.Sprintf("%s/oauth2/clients/%s", baseURL, registration.ClientID)
		resp := doRequest(ctx, t, http.MethodGet, uri, nil, bearer(registration.RegistrationAccessToken))
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		requireNoStore(t, resp)
	})

	t.Run("PutClientConfiguration", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		registration := registerClient(ctx, t, client)
		body := registrationBody(t, codersdk.OAuth2ClientRegistrationRequest{
			RedirectURIs: []string{"https://example.com/updated-callback"},
			ClientName:   fmt.Sprintf("nostore-updated-%s", testutil.MustRandString(t, 10)),
		})

		uri := fmt.Sprintf("%s/oauth2/clients/%s", baseURL, registration.ClientID)
		resp := doRequest(ctx, t, http.MethodPut, uri, strings.NewReader(body), jsonContentType, bearer(registration.RegistrationAccessToken))
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		requireNoStore(t, resp)
	})

	t.Run("DeleteClientConfiguration", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		registration := registerClient(ctx, t, client)

		// RFC 7592 §2.3's own example shows no-store on exactly this 204.
		uri := fmt.Sprintf("%s/oauth2/clients/%s", baseURL, registration.ClientID)
		resp := doRequest(ctx, t, http.MethodDelete, uri, nil, bearer(registration.RegistrationAccessToken))
		defer resp.Body.Close()
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		requireNoStore(t, resp)
	})

	t.Run("UnmatchedPath", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// Chi runs a subrouter's middleware chain around its unmatched-path
		// handling, so a path with no route still carries the headers. The
		// status is not asserted: the request falls through to the root
		// router's SPA handler, which is not this middleware's business.
		resp := doRequest(ctx, t, http.MethodGet, baseURL+"/oauth2/does-not-exist", nil)
		defer resp.Body.Close()
		requireNoStore(t, resp)
	})

	// Discovery metadata is public and RFC 9728 §5 asks for it to be
	// cacheable. These do not prove it is, since the endpoints advertise no
	// freshness lifetime; they pin the exclusion of this middleware.
	for _, path := range []string{
		"/.well-known/oauth-authorization-server",
		"/.well-known/oauth-protected-resource",
	} {
		t.Run("Cacheable"+path, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			resp := doRequest(ctx, t, http.MethodGet, baseURL+path, nil)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.NotContains(t, resp.Header.Get("Cache-Control"), "no-store")
		})
	}
}

// TestOAuth2ProviderNoStoreHeaders asserts the same headers on the
// /api/v2/oauth2-provider tree, where POST /apps/{app}/secrets returns
// ClientSecretFull in plaintext and so meets RFC 6749 §5.1's predicate as
// squarely as anything under /oauth2.
func TestOAuth2ProviderNoStoreHeaders(t *testing.T) {
	t.Parallel()

	client := coderdtest.New(t, nil)
	_ = coderdtest.CreateFirstUser(t, client)
	baseURL := client.URL.String()

	t.Run("CreateAppSecret", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		app, _ := oauth2providertest.CreateTestOAuth2App(t, client)

		uri := fmt.Sprintf("%s/api/v2/oauth2-provider/apps/%s/secrets", baseURL, app.ID)
		resp := doRequest(ctx, t, http.MethodPost, uri, nil, sessionToken(client))
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		requireNoStore(t, resp)

		var secret codersdk.OAuth2ProviderAppSecretFull
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&secret))
		require.NotEmpty(t, secret.ClientSecretFull)
	})

	t.Run("ListApps", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)

		// A credential-free route still carries the headers, which shows the
		// mount is on the tree rather than on one handler.
		resp := doRequest(ctx, t, http.MethodGet, baseURL+"/api/v2/oauth2-provider/apps", nil, sessionToken(client))
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		requireNoStore(t, resp)
	})
}

func requireNoStore(t *testing.T, resp *http.Response) {
	t.Helper()

	require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	require.Equal(t, "no-cache", resp.Header.Get("Pragma"))
}

// doRequest performs a request without following redirects, so a 302's own
// headers can be asserted rather than the redirect target's.
func doRequest(ctx context.Context, t *testing.T, method, uri string, body io.Reader, opts ...func(*http.Request)) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, method, uri, body)
	require.NoError(t, err)
	for _, opt := range opts {
		opt(req)
	}

	httpClient := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := httpClient.Do(req)
	require.NoError(t, err)

	return resp
}

func formContentType(r *http.Request) {
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
}

func jsonContentType(r *http.Request) {
	r.Header.Set("Content-Type", "application/json")
}

func sessionToken(client *codersdk.Client) func(*http.Request) {
	return func(r *http.Request) {
		r.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
	}
}

func bearer(token string) func(*http.Request) {
	return func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	}
}

func authorizeURL(baseURL, clientID, challenge string) string {
	query := url.Values{}
	query.Set("client_id", clientID)
	query.Set("response_type", "code")
	query.Set("redirect_uri", oauth2providertest.TestRedirectURI)
	query.Set("state", "state")
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")

	return baseURL + "/oauth2/authorize?" + query.Encode()
}

func authorizationCode(t *testing.T, client *codersdk.Client, baseURL, clientID, challenge string) string {
	t.Helper()

	state := oauth2providertest.GenerateState(t)
	return oauth2providertest.AuthorizeOAuth2App(t, client, baseURL, oauth2providertest.AuthorizeParams{
		ClientID:            clientID,
		ResponseType:        "code",
		RedirectURI:         oauth2providertest.TestRedirectURI,
		State:               state,
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
	})
}

func registrationBody(t *testing.T, req codersdk.OAuth2ClientRegistrationRequest) string {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)

	return string(body)
}

func registerClient(ctx context.Context, t *testing.T, client *codersdk.Client) codersdk.OAuth2ClientRegistrationResponse {
	t.Helper()

	registration, err := client.PostOAuth2ClientRegistration(ctx, codersdk.OAuth2ClientRegistrationRequest{
		RedirectURIs: []string{"https://example.com/callback"},
		ClientName:   fmt.Sprintf("nostore-client-%s", testutil.MustRandString(t, 10)),
	})
	require.NoError(t, err)

	return registration
}
