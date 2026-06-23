package workspaceapps_test

// App tests can be found in the apptest package.

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/testutil"
)

type fakeSignedTokenProvider struct {
	fromRequestCalls int
	issueCalls       int
}

func (s *fakeSignedTokenProvider) FromRequest(_ *http.Request) (*workspaceapps.SignedToken, bool) {
	s.fromRequestCalls++
	return nil, false
}

func (s *fakeSignedTokenProvider) Issue(_ context.Context, _ http.ResponseWriter, _ *http.Request, _ workspaceapps.IssueTokenRequest) (*workspaceapps.SignedToken, string, bool) {
	s.issueCalls++
	return nil, "", false
}

func TestHandleSubdomain_IgnoresUntrustedForwardedHost(t *testing.T) {
	t.Parallel()

	hostnamePattern := "*--apps.test.coder.com"
	hostnameRegex, err := appurl.CompileHostnamePattern(hostnamePattern)
	require.NoError(t, err)

	dashboardURL, err := url.Parse("https://dashboard.test.coder.com")
	require.NoError(t, err)

	provider := &fakeSignedTokenProvider{}
	srv := workspaceapps.NewServer(workspaceapps.ServerOptions{
		Logger:        testutil.Logger(t),
		DashboardURL:  dashboardURL,
		AccessURL:     dashboardURL,
		Hostname:      hostnamePattern,
		HostnameRegex: hostnameRegex,
		RealIPConfig: &httpmw.RealIPConfig{
			TrustedOrigins: []*net.IPNet{{
				IP:   net.ParseIP("10.0.0.1"),
				Mask: net.CIDRMask(32, 32),
			}},
		},
		SignedTokenProvider: provider,
	})

	forgedHost := appurl.ApplicationURL{
		AppSlugOrPort: "app",
		WorkspaceName: "workspace",
		Username:      "victim",
	}.String() + "--apps.test.coder.com"

	nextCalled := false
	next := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		nextCalled = true
	})

	// Given: a request with a forged X-Forwarded-Host set to a valid
	// app hostname, and an immediate peer outside the trusted proxy
	// config.
	req := httptest.NewRequest(http.MethodGet, "https://dashboard.test.coder.com/", nil)
	req.Header.Set(httpapi.XForwardedHostHeader, forgedHost)
	req.RemoteAddr = "17.18.19.20:1234"

	// When: HandleSubdomain runs.
	srv.HandleSubdomain()(next).ServeHTTP(httptest.NewRecorder(), req)

	// Then: it ignores untrusted X-Forwarded-Host, so the received
	// dashboard host is used, the request falls through to the next
	// handler, and the signed app token provider is never called.
	require.True(t, nextCalled)
	require.Zero(t, provider.fromRequestCalls)
	require.Zero(t, provider.issueCalls)
}

// newSmugglingTestServer builds a workspaceapps.Server wired with a static
// encryption keycache and returns the server, a valid app subdomain host, and a
// freshly minted smuggled API key that decrypts against the server. Each caller
// gets its own server so parallel subtests never share mutable state.
func newSmugglingTestServer(t *testing.T) (srv *workspaceapps.Server, host, encryptedAPIKey string) {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitShort)

	hostnamePattern := "*--apps.test.coder.com"
	hostnameRegex, err := appurl.CompileHostnamePattern(hostnamePattern)
	require.NoError(t, err)

	dashboardURL, err := url.Parse("https://dashboard.test.coder.com")
	require.NoError(t, err)

	// StaticKey satisfies cryptokeys.EncryptionKeycache and lets us mint a valid
	// smuggled API key so the handler reaches the redirect. A256GCMKW needs a
	// 32-byte key.
	keycache := jwtutils.StaticKey{
		ID:  "test",
		Key: generateSecret(t, 32),
	}

	payload := workspaceapps.EncryptedAPIKeyPayload{APIKey: "fake-api-key"}
	payload.Fill(time.Now())
	encryptedAPIKey, err = jwtutils.Encrypt(ctx, keycache, payload)
	require.NoError(t, err)

	srv = workspaceapps.NewServer(workspaceapps.ServerOptions{
		Logger:        testutil.Logger(t),
		DashboardURL:  dashboardURL,
		AccessURL:     dashboardURL,
		Hostname:      hostnamePattern,
		HostnameRegex: hostnameRegex,
		RealIPConfig: &httpmw.RealIPConfig{
			TrustedOrigins: []*net.IPNet{{
				IP:   net.ParseIP("10.0.0.1"),
				Mask: net.CIDRMask(32, 32),
			}},
		},
		SignedTokenProvider:      &fakeSignedTokenProvider{},
		APIKeyEncryptionKeycache: keycache,
	})

	host = appurl.ApplicationURL{
		AppSlugOrPort: "app",
		WorkspaceName: "workspace",
		Username:      "user",
	}.String() + "--apps.test.coder.com"

	return srv, host, encryptedAPIKey
}

// doSmugglingRequest drives HandleSubdomain with a smuggled API key and the
// given (already attacker-decoded) request path. It models how the path
// survives to the handler by setting r.URL.Path directly: the slash-collapsing
// middleware writes to chi's RoutePath, not r.URL.Path. A benign "x=1" query
// parameter rides along to verify it is preserved across the redirect.
func doSmugglingRequest(t *testing.T, srv *workspaceapps.Server, host, encryptedAPIKey, rawPath string) (status int, location string, nextCalled bool) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "https://"+host+"/", nil)
	req.Host = host
	req.RemoteAddr = "10.0.0.1:1234"
	req.URL.Path = rawPath
	req.URL.RawQuery = url.Values{
		workspaceapps.SubdomainProxyAPIKeyParam: {encryptedAPIKey},
		"x":                                     {"1"},
	}.Encode()

	rec := httptest.NewRecorder()
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})

	srv.HandleSubdomain()(next).ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()
	return res.StatusCode, res.Header.Get("Location"), called
}

// TestHandleSubdomain_APIKeySmuggling_NoOpenRedirect ensures that the redirect
// issued after consuming a smuggled subdomain API key cannot be turned into an
// open redirect to an arbitrary external origin (ANT-2026-22456).
//
// The request path is attacker-controlled and already percent-decoded.
// http.Redirect parses the Location with url.Parse (which treats a leading "//"
// as a scheme-relative URL) and emits it verbatim when parsing fails. Browsers
// additionally strip tab/newline characters and treat a leading "/\" like "//",
// so a raw tab such as "/<tab>/evil.com" is a real bypass over HTTP/1 because
// the header writer does not sanitize tabs.
func TestHandleSubdomain_APIKeySmuggling_NoOpenRedirect(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// path is the already-decoded value of r.URL.Path as seen by the handler.
		path string
	}{
		{name: "DoubleSlash", path: "//evil.com/phish"},
		{name: "TripleSlash", path: "///evil.com/phish"},
		{name: "LeadingBackslash", path: "/\\evil.com/phish"},
		{name: "LeadingTab", path: "/\t/evil.com/phish"},
		{name: "TabThenBackslash", path: "/\t\\evil.com/phish"},
		{name: "LeadingBackslashSlash", path: "/\\/evil.com/phish"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv, host, encryptedAPIKey := newSmugglingTestServer(t)

			status, loc, nextCalled := doSmugglingRequest(t, srv, host, encryptedAPIKey, tc.path)

			// The smuggled key is consumed and a redirect is issued instead of
			// falling through to the proxied app.
			require.Equal(t, http.StatusSeeOther, status)
			require.False(t, nextCalled)
			require.NotEmpty(t, loc)

			// Model browser URL normalization: tab/newline/CR are stripped and a
			// backslash is treated like a forward slash before the URL is
			// resolved. After that, the Location must not look like a
			// scheme-relative ("//host") URL.
			browserLoc := strings.NewReplacer("\t", "", "\n", "", "\r", "").Replace(loc)
			browserLoc = strings.ReplaceAll(browserLoc, "\\", "/")
			require.False(t, strings.HasPrefix(browserLoc, "//"),
				"redirect %q resolves off-origin (browser-normalized to %q)", loc, browserLoc)

			// Go's url.Parse must also see a same-origin, relative reference.
			parsed, err := url.Parse(loc)
			require.NoError(t, err)
			require.Empty(t, parsed.Scheme, "redirect %q must not carry a scheme", loc)
			require.Empty(t, parsed.Host, "redirect %q must not carry a host", loc)

			// The smuggled key is stripped and unrelated query params survive.
			require.Empty(t, parsed.Query().Get(workspaceapps.SubdomainProxyAPIKeyParam))
			require.Equal(t, "1", parsed.Query().Get("x"))
		})
	}
}

// TestHandleSubdomain_APIKeySmuggling_PreservesPath ensures the security fix does
// not change the redirect for legitimate paths: the path round-trips and the
// smuggled key is stripped while other query params are preserved.
func TestHandleSubdomain_APIKeySmuggling_PreservesPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		path     string
		wantPath string
	}{
		{name: "Empty", path: "", wantPath: "/"},
		{name: "Root", path: "/", wantPath: "/"},
		{name: "Simple", path: "/test", wantPath: "/test"},
		{name: "Nested", path: "/app/sub/page", wantPath: "/app/sub/page"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv, host, encryptedAPIKey := newSmugglingTestServer(t)

			status, loc, nextCalled := doSmugglingRequest(t, srv, host, encryptedAPIKey, tc.path)

			require.Equal(t, http.StatusSeeOther, status)
			require.False(t, nextCalled)

			parsed, err := url.Parse(loc)
			require.NoError(t, err)
			require.Equal(t, tc.wantPath, parsed.Path)
			require.Empty(t, parsed.Host)
			require.Empty(t, parsed.Query().Get(workspaceapps.SubdomainProxyAPIKeyParam))
			require.Equal(t, "1", parsed.Query().Get("x"))
		})
	}
}
