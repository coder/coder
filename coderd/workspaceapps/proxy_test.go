package workspaceapps_test

// App tests can be found in the apptest package.

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
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
