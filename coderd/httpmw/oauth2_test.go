package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type testOAuth2Provider struct {
	t        testing.TB
	authOpts []oauth2.AuthCodeOption
}

func (p *testOAuth2Provider) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	assert.EqualValues(p.t, p.authOpts, opts)
	return "?state=" + url.QueryEscape(state)
}

func (*testOAuth2Provider) Exchange(_ context.Context, _ string, _ ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken: "hello",
	}, nil
}

func (*testOAuth2Provider) TokenSource(_ context.Context, _ *oauth2.Token) oauth2.TokenSource {
	return nil
}

func newTestOAuth2Provider(t testing.TB, opts ...oauth2.AuthCodeOption) *testOAuth2Provider {
	return &testOAuth2Provider{
		t:        t,
		authOpts: opts,
	}
}

// nolint:bodyclose
func TestOAuth2(t *testing.T) {
	t.Parallel()
	t.Run("NotSetup", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/", nil)
		res := httptest.NewRecorder()
		httpmw.ExtractOAuth2(nil, nil, codersdk.HTTPCookieConfig{}, nil, nil, nil, "")(nil).ServeHTTP(res, req)
		require.Equal(t, http.StatusBadRequest, res.Result().StatusCode)
	})
	t.Run("RedirectWithoutCode", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/?redirect="+url.QueryEscape("/dashboard"), nil)
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil, nil, "")(nil).ServeHTTP(res, req)
		location := res.Header().Get("Location")
		if !assert.NotEmpty(t, location) {
			return
		}
		require.Len(t, res.Result().Cookies(), 2)
		cookie := res.Result().Cookies()[1]
		require.Equal(t, "/dashboard", cookie.Value)
	})
	t.Run("OnlyPathBaseRedirect", func(t *testing.T) {
		t.Parallel()
		// Construct a URI to a potentially malicious
		// site and assert that we omit the host
		// when redirecting the request.
		uri := &url.URL{
			Scheme:   "https",
			Host:     "some.bad.domain.com",
			Path:     "/sadf/asdfasdf",
			RawQuery: "foo=hello&bar=world",
		}
		expectedValue := uri.Path + "?" + uri.RawQuery
		req := httptest.NewRequest("GET", "/?redirect="+url.QueryEscape(uri.String()), nil)
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil, nil, "")(nil).ServeHTTP(res, req)
		location := res.Header().Get("Location")
		if !assert.NotEmpty(t, location) {
			return
		}
		require.Len(t, res.Result().Cookies(), 2)
		cookie := res.Result().Cookies()[1]
		require.Equal(t, expectedValue, cookie.Value)
	})

	t.Run("NoState", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/?code=something", nil)
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil, nil, "")(nil).ServeHTTP(res, req)
		require.Equal(t, http.StatusBadRequest, res.Result().StatusCode)
	})
	t.Run("NoStateCookie", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/?code=something&state=test", nil)
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil, nil, "")(nil).ServeHTTP(res, req)
		require.Equal(t, http.StatusUnauthorized, res.Result().StatusCode)
	})
	t.Run("MismatchedState", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/?code=something&state=test", nil)
		req.AddCookie(&http.Cookie{
			Name:  codersdk.OAuth2StateCookie,
			Value: "mismatch",
		})
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil, nil, "")(nil).ServeHTTP(res, req)
		require.Equal(t, http.StatusUnauthorized, res.Result().StatusCode)
	})
	t.Run("ExchangeCodeAndState", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/?code=test&state=something", nil)
		req.AddCookie(&http.Cookie{
			Name:  codersdk.OAuth2StateCookie,
			Value: "something",
		})
		req.AddCookie(&http.Cookie{
			Name:  "oauth_redirect",
			Value: "/dashboard",
		})
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil, nil, "")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			state := httpmw.OAuth2(r)
			require.Equal(t, "/dashboard", state.Redirect)
		})).ServeHTTP(res, req)
	})
	t.Run("CustomAuthCodeOptions", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/?redirect="+url.QueryEscape("/dashboard"), nil)
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("foo", "bar"))
		authOpts := map[string]string{"foo": "bar"}
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, authOpts, nil, nil, "")(nil).ServeHTTP(res, req)
		location := res.Header().Get("Location")
		// Ideally we would also assert that the location contains the query params
		// we set in the auth URL but this would essentially be testing the oauth2 package.
		// testOAuth2Provider does this job for us.
		require.NotEmpty(t, location)
	})
	t.Run("PresetConvertState", func(t *testing.T) {
		t.Parallel()
		customState := testutil.GetRandomName(t)
		req := httptest.NewRequest("GET", "/?oidc_merge_state="+customState+"&redirect="+url.QueryEscape("/dashboard"), nil)
		res := httptest.NewRecorder()
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{
			Secure:   true,
			SameSite: "none",
		}, nil, nil, nil, "")(nil).ServeHTTP(res, req)

		found := false
		for _, cookie := range res.Result().Cookies() {
			if cookie.Name == codersdk.OAuth2StateCookie {
				require.Equal(t, cookie.Value, customState, "expected state")
				require.Equal(t, true, cookie.Secure, "cookie set to secure")
				require.Equal(t, http.SameSiteNoneMode, cookie.SameSite, "same-site = none")
				found = true
			}
		}
		require.True(t, found, "expected state cookie")
	})
}

// nolint:bodyclose
func TestOAuth2DynamicRedirect(t *testing.T) {
	t.Parallel()

	const callbackPath = "/api/v2/users/oidc/callback"
	const primaryHost = "coder.test.netflix.net"
	const altHost = "dev-workspaces.test.netflix.net"
	const wantPrimaryURI = "https://" + primaryHost + callbackPath
	const wantAltURI = "https://" + altHost + callbackPath

	t.Run("InitOnAllowedHostSetsCookieAndOverridesRedirect", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", callbackPath+"?redirect="+url.QueryEscape("/dashboard"), nil)
		req.Host = altHost
		res := httptest.NewRecorder()

		tp := newTestOAuth2Provider(t,
			oauth2.AccessTypeOffline,
			oauth2.SetAuthURLParam("redirect_uri", wantAltURI),
		)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil,
			[]string{primaryHost, altHost}, "https")(nil).ServeHTTP(res, req)

		require.Equal(t, http.StatusTemporaryRedirect, res.Result().StatusCode)

		var redirectCookie *http.Cookie
		for _, c := range res.Result().Cookies() {
			if c.Name == codersdk.OAuth2RedirectURICookie {
				redirectCookie = c
				break
			}
		}
		require.NotNil(t, redirectCookie, "expected %s cookie", codersdk.OAuth2RedirectURICookie)
		require.Equal(t, wantAltURI, redirectCookie.Value)
	})

	t.Run("InitOnDisallowedHostReturnsBadRequest", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", callbackPath, nil)
		req.Host = "evil.example.com"
		res := httptest.NewRecorder()

		// authOpts must not be asserted: the request should be rejected
		// before AuthCodeURL is called.
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil,
			[]string{primaryHost, altHost}, "")(nil).ServeHTTP(res, req)

		require.Equal(t, http.StatusBadRequest, res.Result().StatusCode)
		for _, c := range res.Result().Cookies() {
			require.NotEqual(t, codersdk.OAuth2RedirectURICookie, c.Name, "must not set redirect_uri cookie when host is rejected")
		}
	})

	t.Run("AllowlistHostMatchIsCaseInsensitiveAndIgnoresPort", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", callbackPath, nil)
		req.Host = "DEV-WORKSPACES.test.netflix.net:8443"
		res := httptest.NewRecorder()

		// Host is preserved verbatim in the constructed redirect_uri so the
		// IdP sees exactly what the user typed (case is preserved but the
		// allowlist match is insensitive). The scheme comes from the caller-
		// supplied defaultScheme; real callers populate this from AccessURL.
		expectedURI := "https://DEV-WORKSPACES.test.netflix.net:8443" + callbackPath
		tp := newTestOAuth2Provider(t,
			oauth2.AccessTypeOffline,
			oauth2.SetAuthURLParam("redirect_uri", expectedURI),
		)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil,
			[]string{altHost}, "https")(nil).ServeHTTP(res, req)

		require.Equal(t, http.StatusTemporaryRedirect, res.Result().StatusCode)
	})

	t.Run("ExchangeReusesRedirectURIFromCookie", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", callbackPath+"?code=test&state=something", nil)
		req.Host = altHost
		req.AddCookie(&http.Cookie{Name: codersdk.OAuth2StateCookie, Value: "something"})
		req.AddCookie(&http.Cookie{Name: codersdk.OAuth2RedirectCookie, Value: "/dashboard"})
		req.AddCookie(&http.Cookie{Name: codersdk.OAuth2RedirectURICookie, Value: wantAltURI})
		res := httptest.NewRecorder()

		exchangeCalled := false
		tp := &exchangeAssertingProvider{
			t: t,
			onExchange: func(opts []oauth2.AuthCodeOption) {
				exchangeCalled = true
				require.Contains(t, opts, oauth2.SetAuthURLParam("redirect_uri", wantAltURI))
			},
		}
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil,
			[]string{primaryHost, altHost}, "https")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			state := httpmw.OAuth2(r)
			require.Equal(t, "/dashboard", state.Redirect)
		})).ServeHTTP(res, req)
		require.True(t, exchangeCalled, "expected Exchange to be invoked")
	})

	t.Run("CallbackWithMissingRedirectURICookieReturnsBadRequest", func(t *testing.T) {
		t.Parallel()
		// Same shape as ExchangeReusesRedirectURIFromCookie but without the
		// redirect_uri cookie. Must fail loudly rather than silently sending
		// the static config redirect_uri (which would mismatch what was used
		// in the original authorization request).
		req := httptest.NewRequest("GET", callbackPath+"?code=test&state=something", nil)
		req.Host = altHost
		req.AddCookie(&http.Cookie{Name: codersdk.OAuth2StateCookie, Value: "something"})
		req.AddCookie(&http.Cookie{Name: codersdk.OAuth2RedirectCookie, Value: "/dashboard"})
		// Intentionally NO OAuth2RedirectURICookie.
		res := httptest.NewRecorder()

		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil,
			[]string{primaryHost, altHost}, "https")(nil).ServeHTTP(res, req)

		require.Equal(t, http.StatusBadRequest, res.Result().StatusCode)
	})

	t.Run("CallbackWithMismatchedRedirectURICookieReturnsBadRequest", func(t *testing.T) {
		t.Parallel()
		// Cookie was set when the user initiated on altHost, but the callback
		// is somehow arriving from primaryHost (or the cookie was tampered).
		// Defense in depth: reject the exchange instead of forwarding a
		// stale/mismatched value to the IdP.
		req := httptest.NewRequest("GET", callbackPath+"?code=test&state=something", nil)
		req.Host = primaryHost
		req.AddCookie(&http.Cookie{Name: codersdk.OAuth2StateCookie, Value: "something"})
		req.AddCookie(&http.Cookie{Name: codersdk.OAuth2RedirectURICookie, Value: wantAltURI})
		res := httptest.NewRecorder()

		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil,
			[]string{primaryHost, altHost}, "https")(nil).ServeHTTP(res, req)

		require.Equal(t, http.StatusBadRequest, res.Result().StatusCode)
	})

	t.Run("CallbackOnDisallowedHostReturnsBadRequest", func(t *testing.T) {
		t.Parallel()
		// Even with a valid state cookie and code, the host must be allowed.
		req := httptest.NewRequest("GET", callbackPath+"?code=test&state=something", nil)
		req.Host = "evil.example.com"
		req.AddCookie(&http.Cookie{Name: codersdk.OAuth2StateCookie, Value: "something"})
		req.AddCookie(&http.Cookie{Name: codersdk.OAuth2RedirectURICookie, Value: wantPrimaryURI})
		res := httptest.NewRecorder()

		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil,
			[]string{primaryHost}, "")(nil).ServeHTTP(res, req)

		require.Equal(t, http.StatusBadRequest, res.Result().StatusCode)
	})

	t.Run("AllowlistDisabledLeavesBehaviorUnchanged", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", callbackPath+"?redirect="+url.QueryEscape("/dashboard"), nil)
		req.Host = "anything.example.com"
		res := httptest.NewRecorder()

		// With no allowlist, AuthCodeURL must be invoked with only the base
		// AccessTypeOffline option; no redirect_uri override should be added.
		tp := newTestOAuth2Provider(t, oauth2.AccessTypeOffline)
		httpmw.ExtractOAuth2(tp, nil, codersdk.HTTPCookieConfig{}, nil, nil, nil, "")(nil).ServeHTTP(res, req)

		require.Equal(t, http.StatusTemporaryRedirect, res.Result().StatusCode)
		for _, c := range res.Result().Cookies() {
			require.NotEqual(t, codersdk.OAuth2RedirectURICookie, c.Name)
		}
	})
}

// exchangeAssertingProvider is a test OAuth2 provider that captures the
// options passed to Exchange so the test can assert on them.
type exchangeAssertingProvider struct {
	t          testing.TB
	onExchange func(opts []oauth2.AuthCodeOption)
}

func (*exchangeAssertingProvider) AuthCodeURL(state string, _ ...oauth2.AuthCodeOption) string {
	return "?state=" + url.QueryEscape(state)
}

func (p *exchangeAssertingProvider) Exchange(_ context.Context, _ string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	if p.onExchange != nil {
		p.onExchange(opts)
	}
	return &oauth2.Token{AccessToken: "hello"}, nil
}

func (*exchangeAssertingProvider) TokenSource(_ context.Context, _ *oauth2.Token) oauth2.TokenSource {
	return nil
}
