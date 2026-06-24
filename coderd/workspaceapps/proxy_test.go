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

// TestHandleSubdomain_APIKeySmuggling_NoOpenRedirect verifies that after
// HandleSubdomain consumes a smuggled subdomain API key, the redirect it issues
// to strip the key stays on the current origin and cannot be pointed at an
// external host (ANT-2026-22456).
//
// This is a focused wiring test: it confirms the real handler sanitizes the
// redirect target and strips the key while preserving other query parameters.
// The exhaustive path-sanitization matrix (slash, backslash, tab, newline, CR,
// encoded variants) lives in Test_originLocalPath, which exercises the same
// url.URL construction without the server setup. It is built inline like the
// sibling test above; there is no lighter shared fixture, and the full app test
// harness cannot model an attacker path that bypasses slash-cleaning middleware.
func TestHandleSubdomain_APIKeySmuggling_NoOpenRedirect(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)

	hostnamePattern := "*--apps.test.coder.com"
	hostnameRegex, err := appurl.CompileHostnamePattern(hostnamePattern)
	require.NoError(t, err)

	dashboardURL, err := url.Parse("https://dashboard.test.coder.com")
	require.NoError(t, err)

	// StaticKey satisfies cryptokeys.EncryptionKeycache and lets us mint a valid
	// smuggled API key so the handler reaches the redirect. A256GCMKW needs a
	// 32-byte key.
	keycache := jwtutils.StaticKey{ID: "test", Key: generateSecret(t, 32)}
	payload := workspaceapps.EncryptedAPIKeyPayload{APIKey: "fake-api-key"}
	payload.Fill(time.Now())
	encryptedAPIKey, err := jwtutils.Encrypt(ctx, keycache, payload)
	require.NoError(t, err)

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
		SignedTokenProvider:      &fakeSignedTokenProvider{},
		APIKeyEncryptionKeycache: keycache,
	})

	host := appurl.ApplicationURL{
		AppSlugOrPort: "app",
		WorkspaceName: "workspace",
		Username:      "user",
	}.String() + "--apps.test.coder.com"

	// The HTTP server populates r.URL.Path from the request line; set it directly
	// to model the attacker-supplied "//evil.com" path that survives to the
	// handler (the slash-collapsing middleware writes to chi's RoutePath, not
	// r.URL.Path). A benign "x=1" rides along to confirm it is preserved.
	req := httptest.NewRequest(http.MethodGet, "https://"+host+"/", nil)
	req.Host = host
	req.RemoteAddr = "10.0.0.1:1234"
	req.URL.Path = "//evil.com/phish"
	req.URL.RawQuery = url.Values{
		workspaceapps.SubdomainProxyAPIKeyParam: {encryptedAPIKey},
		"x":                                     {"1"},
	}.Encode()

	rec := httptest.NewRecorder()
	nextCalled := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	})

	srv.HandleSubdomain()(next).ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	// The smuggled key is consumed and a redirect is issued instead of falling
	// through to the proxied app.
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.False(t, nextCalled)

	loc := res.Header.Get("Location")
	require.NotEmpty(t, loc)
	require.False(t, strings.HasPrefix(loc, "//"), "redirect %q must stay same-origin", loc)

	parsed, err := url.Parse(loc)
	require.NoError(t, err)
	require.Empty(t, parsed.Scheme, "redirect %q must not carry a scheme", loc)
	require.Empty(t, parsed.Host, "redirect %q must not carry a host", loc)

	// The smuggled key is stripped and unrelated query params survive.
	require.Empty(t, parsed.Query().Get(workspaceapps.SubdomainProxyAPIKeyParam))
	require.Equal(t, "1", parsed.Query().Get("x"))
}
