package coderd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type embedAuthFixture struct {
	ownerClient       *codersdk.Client
	ownerID           string
	ownerToken        string
	iframeClient      *codersdk.Client
	sessionCookieName string
}

func TestEmbedAuth(t *testing.T) {
	t.Parallel()

	t.Run("MissingToken", func(t *testing.T) {
		t.Parallel()
		f := newEmbedAuthFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		res, jar := postEmbedSessionJSON(ctx, t, f.iframeClient, codersdk.EmbedSessionTokenRequest{})
		defer res.Body.Close()

		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		assertNoSessionCookie(t, res.Cookies(), f.sessionCookieName)
		assertNoSessionCookie(t, jar.Cookies(f.iframeClient.URL), f.sessionCookieName)
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		t.Parallel()
		f := newEmbedAuthFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		res, jar := postEmbedSessionRaw(ctx, t, f.iframeClient, bytes.NewBufferString(`{"token":`))
		defer res.Body.Close()

		require.Equal(t, http.StatusBadRequest, res.StatusCode)
		assertNoSessionCookie(t, res.Cookies(), f.sessionCookieName)
		assertNoSessionCookie(t, jar.Cookies(f.iframeClient.URL), f.sessionCookieName)
	})

	t.Run("InvalidToken", func(t *testing.T) {
		t.Parallel()
		f := newEmbedAuthFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		res, jar := postEmbedSessionJSON(ctx, t, f.iframeClient, codersdk.EmbedSessionTokenRequest{Token: "bogus-token"})
		defer res.Body.Close()

		require.Equal(t, http.StatusUnauthorized, res.StatusCode)
		assertNoSessionCookie(t, res.Cookies(), f.sessionCookieName)
		assertNoSessionCookie(t, jar.Cookies(f.iframeClient.URL), f.sessionCookieName)
	})

	t.Run("HappyPathSetsEmbedCookie", func(t *testing.T) {
		t.Parallel()
		f := newEmbedAuthFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		res, _ := postEmbedSessionJSON(ctx, t, f.iframeClient, codersdk.EmbedSessionTokenRequest{Token: f.ownerToken})
		defer res.Body.Close()

		require.Equal(t, http.StatusNoContent, res.StatusCode)
		cookie := requireSessionCookie(t, res.Cookies(), f.sessionCookieName)
		require.Equal(t, f.ownerToken, cookie.Value)
		require.True(t, cookie.HttpOnly)
		require.False(t, cookie.Secure)
		require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
		require.Equal(t, "/", cookie.Path)
	})

	t.Run("CookieAuthenticatesUsersMe", func(t *testing.T) {
		t.Parallel()
		f := newEmbedAuthFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		bootstrapEmbedSession(ctx, t, f)

		user, err := f.iframeClient.User(ctx, codersdk.Me)
		require.NoError(t, err)
		require.Equal(t, f.ownerID, user.ID.String())
	})

	t.Run("LogoutClearsSession", func(t *testing.T) {
		t.Parallel()
		f := newEmbedAuthFixture(t)
		ctx := testutil.Context(t, testutil.WaitLong)

		bootstrapEmbedSession(ctx, t, f)

		logoutRes := postLogout(ctx, t, f.iframeClient, f.ownerToken)
		defer logoutRes.Body.Close()
		require.Equal(t, http.StatusOK, logoutRes.StatusCode)
		logoutCookie := requireSessionCookie(t, logoutRes.Cookies(), f.sessionCookieName)
		require.Equal(t, -1, logoutCookie.MaxAge)

		staleCookieClient, staleCookieJar := newIframeClient(t, f.ownerClient.URL)
		seedSessionCookieForHTTPTest(t, staleCookieJar, staleCookieClient.URL, &http.Cookie{
			Name:  f.sessionCookieName,
			Value: f.ownerToken,
			Path:  "/",
		})

		_, err := staleCookieClient.User(ctx, codersdk.Me)
		require.Error(t, err)
		require.Equal(t, http.StatusUnauthorized, coderdtest.SDKError(t, err).StatusCode())
	})
}

func newEmbedAuthFixture(t *testing.T) embedAuthFixture {
	t.Helper()

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentAgents)}
	ownerClient := coderdtest.New(t, &coderdtest.Options{DeploymentValues: dv})
	owner := coderdtest.CreateFirstUser(t, ownerClient)

	iframeClient, _ := newIframeClient(t, ownerClient.URL)
	require.Empty(t, iframeClient.SessionToken())

	return embedAuthFixture{
		ownerClient:       ownerClient,
		ownerID:           owner.UserID.String(),
		ownerToken:        ownerClient.SessionToken(),
		iframeClient:      iframeClient,
		sessionCookieName: effectiveSessionCookieName(dv),
	}
}

func newIframeClient(t *testing.T, serverURL *url.URL) (*codersdk.Client, *cookiejar.Jar) {
	t.Helper()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	iframeClient := codersdk.New(serverURL)
	iframeClient.HTTPClient.Jar = jar
	iframeClient.SetSessionToken("")
	t.Cleanup(func() {
		iframeClient.HTTPClient.CloseIdleConnections()
	})
	return iframeClient, jar
}

func effectiveSessionCookieName(dv *codersdk.DeploymentValues) string {
	cookie := dv.HTTPCookies.Apply(&http.Cookie{Name: codersdk.SessionTokenCookie})
	return cookie.Name
}

func postEmbedSessionJSON(ctx context.Context, t *testing.T, client *codersdk.Client, req codersdk.EmbedSessionTokenRequest) (*http.Response, *cookiejar.Jar) {
	t.Helper()

	body, err := json.Marshal(req)
	require.NoError(t, err)

	return postEmbedSessionRaw(ctx, t, client, bytes.NewReader(body))
}

func postEmbedSessionRaw(ctx context.Context, t *testing.T, client *codersdk.Client, body io.Reader) (*http.Response, *cookiejar.Jar) {
	t.Helper()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	endpoint, err := client.URL.Parse("/api/experimental/chats/embed-session")
	require.NoError(t, err)

	httpClient := &http.Client{Jar: jar}
	t.Cleanup(func() {
		httpClient.CloseIdleConnections()
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	res, err := httpClient.Do(req)
	require.NoError(t, err)
	return res, jar
}

func postLogout(ctx context.Context, t *testing.T, client *codersdk.Client, token string) *http.Response {
	t.Helper()
	require.NotEmpty(t, token)

	endpoint, err := client.URL.Parse("/api/v2/users/logout")
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), nil)
	require.NoError(t, err)
	req.Header.Set(codersdk.SessionTokenHeader, token)

	res, err := client.HTTPClient.Do(req)
	require.NoError(t, err)
	return res
}

func bootstrapEmbedSession(ctx context.Context, t *testing.T, f embedAuthFixture) {
	t.Helper()

	res, jar := postEmbedSessionJSON(ctx, t, f.iframeClient, codersdk.EmbedSessionTokenRequest{Token: f.ownerToken})
	defer res.Body.Close()

	require.Equal(t, http.StatusNoContent, res.StatusCode)
	cookie := requireSessionCookie(t, res.Cookies(), f.sessionCookieName)
	require.Equal(t, f.ownerToken, cookie.Value)

	storedCookie := requireSessionCookie(t, jar.Cookies(f.iframeClient.URL), f.sessionCookieName)
	require.Equal(t, cookie.Value, storedCookie.Value)
	f.iframeClient.HTTPClient.Jar = jar
}

func seedSessionCookieForHTTPTest(t *testing.T, jar *cookiejar.Jar, serverURL *url.URL, cookie *http.Cookie) {
	t.Helper()
	require.NotNil(t, jar)
	require.NotNil(t, serverURL)
	require.NotNil(t, cookie)

	// Keep only the fields needed to exercise HTTP test requests. A Secure
	// cookie would be dropped by net/http/cookiejar for http:// URLs.
	jar.SetCookies(serverURL, []*http.Cookie{{
		Name:  cookie.Name,
		Value: cookie.Value,
		Path:  cookie.Path,
	}})
}

func requireSessionCookie(t *testing.T, cookies []*http.Cookie, expectedName string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == expectedName {
			return cookie
		}
	}
	require.Failf(t, "session cookie missing", "expected cookie %q in response", expectedName)
	return nil
}

func assertNoSessionCookie(t *testing.T, cookies []*http.Cookie, expectedName string) {
	t.Helper()
	for _, cookie := range cookies {
		require.NotEqual(t, expectedName, cookie.Name)
	}
}
