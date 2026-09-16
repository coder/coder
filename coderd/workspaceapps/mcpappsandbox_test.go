package workspaceapps_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// mcpAppSandboxTestHost is a valid reserved sandbox subdomain under the
// default test wildcard pattern.
const mcpAppSandboxTestHost = workspaceapps.MCPAppSandboxHostPrefix + "0123456789abcdef.apps.test.coder.com"

func newMCPAppSandboxTestServer(t *testing.T, hostnamePattern, dashboardURL string) *workspaceapps.Server {
	t.Helper()

	hostnameRegex, err := appurl.CompileHostnamePattern(hostnamePattern)
	require.NoError(t, err)
	dashboard, err := url.Parse(dashboardURL)
	require.NoError(t, err)

	return workspaceapps.NewServer(workspaceapps.ServerOptions{
		Logger:              testutil.Logger(t),
		DashboardURL:        dashboard,
		AccessURL:           dashboard,
		Hostname:            hostnamePattern,
		HostnameRegex:       hostnameRegex,
		SignedTokenProvider: &fakeSignedTokenProvider{},
	})
}

// doMCPAppSandboxRequest runs a request with the given method, Host and
// request URI through HandleSubdomain and reports whether the next handler
// was reached.
func doMCPAppSandboxRequest(t *testing.T, srv *workspaceapps.Server, method, host, requestURI string) (*httptest.ResponseRecorder, bool) {
	t.Helper()

	req := httptest.NewRequest(method, "https://"+host+requestURI, nil)
	req.Host = host
	req.RemoteAddr = "10.0.0.1:1234"

	nextCalled := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	})
	rec := httptest.NewRecorder()
	srv.HandleSubdomain()(next).ServeHTTP(rec, req)
	return rec, nextCalled
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder) codersdk.Response {
	t.Helper()

	var resp codersdk.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp), "body: %s", rec.Body.String())
	return resp
}

func TestMCPAppSandboxHostRegex(t *testing.T) {
	t.Parallel()

	cases := []struct {
		subdomain string
		want      bool
	}{
		{subdomain: "mcp-0123456789abcdef", want: true},
		{subdomain: "mcp-0000000000000000", want: true},
		{subdomain: "mcp-0123456789ABCDEF", want: false},
		{subdomain: "mcp-0123456789abcde", want: false},
		{subdomain: "mcp-0123456789abcdef0", want: false},
		{subdomain: "mcp-0123456789abcdeg", want: false},
		{subdomain: "mcp-xyz", want: false},
		{subdomain: "mcp-", want: false},
		{subdomain: "mcpapp", want: false},
		{subdomain: "xmcp-0123456789abcdef", want: false},
		{subdomain: "mcp-0123456789abcdef--app", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.subdomain, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, workspaceapps.MCPAppSandboxHostRegex.MatchString(tc.subdomain))
		})
	}
}

func TestServeMCPAppSandbox(t *testing.T) {
	t.Parallel()

	const (
		hostnamePattern = "*.apps.test.coder.com"
		dashboardURL    = "https://dashboard.test.coder.com"
	)

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		srv := newMCPAppSandboxTestServer(t, hostnamePattern, dashboardURL)
		rec, nextCalled := doMCPAppSandboxRequest(t, srv, http.MethodGet, mcpAppSandboxTestHost, "/")
		require.False(t, nextCalled)
		require.Equal(t, http.StatusOK, rec.Code)

		require.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))
		require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
		require.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
		require.Empty(t, rec.Header().Get("X-Frame-Options"))

		csp := rec.Header().Get("Content-Security-Policy")
		require.Equal(t, strings.Join([]string{
			"default-src 'none'",
			"script-src 'self' 'unsafe-inline'",
			"style-src 'self' 'unsafe-inline'",
			"connect-src 'self'",
			"img-src 'self' data:",
			"font-src 'self'",
			"media-src 'self' data:",
			"frame-src 'none'",
			"object-src 'none'",
			"base-uri 'self'",
			"form-action 'none'",
			"frame-ancestors https://dashboard.test.coder.com",
		}, "; "), csp)

		body := rec.Body.String()
		require.Contains(t, body, `<meta name="mcp-app-host-origin" content="https://dashboard.test.coder.com" />`)
		require.Contains(t, body, "ui/notifications/sandbox-proxy-ready")
		require.Contains(t, body, "ui/notifications/sandbox-resource-ready")
		require.NotContains(t, body, "{{")
	})

	t.Run("Head", func(t *testing.T) {
		t.Parallel()

		srv := newMCPAppSandboxTestServer(t, hostnamePattern, dashboardURL)
		rec, nextCalled := doMCPAppSandboxRequest(t, srv, http.MethodHead, mcpAppSandboxTestHost, "/")
		require.False(t, nextCalled)
		require.Equal(t, http.StatusOK, rec.Code)
		require.NotEmpty(t, rec.Header().Get("Content-Security-Policy"))
		require.Empty(t, rec.Body.Bytes())
	})

	t.Run("UppercaseSubdomain", func(t *testing.T) {
		t.Parallel()

		srv := newMCPAppSandboxTestServer(t, hostnamePattern, dashboardURL)
		host := strings.ToUpper(workspaceapps.MCPAppSandboxHostPrefix+"0123456789abcdef") + ".apps.test.coder.com"
		rec, nextCalled := doMCPAppSandboxRequest(t, srv, http.MethodGet, host, "/")
		require.False(t, nextCalled)
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("DashboardWithPort", func(t *testing.T) {
		t.Parallel()

		srv := newMCPAppSandboxTestServer(t, hostnamePattern, "https://dashboard.test.coder.com:8443/some/path")
		rec, _ := doMCPAppSandboxRequest(t, srv, http.MethodGet, mcpAppSandboxTestHost, "/")
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors https://dashboard.test.coder.com:8443")
		require.Contains(t, rec.Body.String(), `content="https://dashboard.test.coder.com:8443"`)
	})

	t.Run("NonRootPath", func(t *testing.T) {
		t.Parallel()

		srv := newMCPAppSandboxTestServer(t, hostnamePattern, dashboardURL)
		for _, path := range []string{"/index.html", "/sw.js", "/a/b", "//"} {
			rec, nextCalled := doMCPAppSandboxRequest(t, srv, http.MethodGet, mcpAppSandboxTestHost, path)
			require.False(t, nextCalled, path)
			require.Equal(t, http.StatusNotFound, rec.Code, path)
			require.Empty(t, rec.Header().Get("Content-Security-Policy"), path)
		}
	})

	t.Run("Post", func(t *testing.T) {
		t.Parallel()

		srv := newMCPAppSandboxTestServer(t, hostnamePattern, dashboardURL)
		rec, nextCalled := doMCPAppSandboxRequest(t, srv, http.MethodPost, mcpAppSandboxTestHost, "/")
		require.False(t, nextCalled)
		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("ParentSuffixRefused", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name      string
			pattern   string
			dashboard string
		}{
			{name: "Under", pattern: "*.example.com", dashboard: "https://coder.example.com"},
			{name: "UnderUppercase", pattern: "*.Example.com", dashboard: "https://CODER.example.com"},
			{name: "DeeplyUnder", pattern: "*.example.com", dashboard: "https://dash.coder.example.com"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				srv := newMCPAppSandboxTestServer(t, tc.pattern, tc.dashboard)
				suffix := strings.ToLower(strings.TrimPrefix(tc.pattern, "*."))
				host := workspaceapps.MCPAppSandboxHostPrefix + "0123456789abcdef." + suffix
				rec, nextCalled := doMCPAppSandboxRequest(t, srv, http.MethodGet, host, "/")
				require.False(t, nextCalled)
				require.Equal(t, http.StatusBadRequest, rec.Code)
				resp := decodeResponse(t, rec)
				require.Contains(t, resp.Detail, "must not be a parent domain")
			})
		}
	})

	t.Run("EqualSuffixAllowed", func(t *testing.T) {
		t.Parallel()

		srv := newMCPAppSandboxTestServer(t, "*.coder.example.com", "https://coder.example.com")
		host := workspaceapps.MCPAppSandboxHostPrefix + "0123456789abcdef.coder.example.com"
		rec, nextCalled := doMCPAppSandboxRequest(t, srv, http.MethodGet, host, "/")
		require.False(t, nextCalled)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors https://coder.example.com")
		require.Contains(t, rec.Body.String(), `content="https://coder.example.com"`)
	})

	t.Run("SiblingSuffixAllowed", func(t *testing.T) {
		t.Parallel()

		srv := newMCPAppSandboxTestServer(t, "*.apps.example.com", "https://coder.example.com")
		host := workspaceapps.MCPAppSandboxHostPrefix + "0123456789abcdef.apps.example.com"
		rec, _ := doMCPAppSandboxRequest(t, srv, http.MethodGet, host, "/")
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("InvalidCSP", func(t *testing.T) {
		t.Parallel()

		tooMany := make([]string, 33)
		for i := range tooMany {
			tooMany[i] = fmt.Sprintf("https://d%d.other.com", i)
		}
		tooManyJSON, err := json.Marshal(map[string]any{"connectDomains": tooMany})
		require.NoError(t, err)

		longDomains := make([]string, 32)
		for i := range longDomains {
			longDomains[i] = "https://" + strings.Repeat("a", 240) + fmt.Sprintf("%d", i) + ".other.com"
		}
		tooLongJSON, err := json.Marshal(map[string]any{"resourceDomains": longDomains})
		require.NoError(t, err)

		cases := []struct {
			name       string
			csp        string
			wantDetail string
		}{
			{name: "NotJSON", csp: "not-json", wantDetail: "invalid character"},
			{name: "WrongType", csp: `{"connectDomains":"https://api.other.com"}`, wantDetail: "cannot unmarshal"},
			{name: "WrongEntryType", csp: `{"connectDomains":[1]}`, wantDetail: "cannot unmarshal"},
			{name: "NullEntry", csp: `{"connectDomains":[null]}`, wantDetail: `connectDomains entry ""`},
			{name: "HTTP", csp: `{"connectDomains":["http://api.other.com"]}`, wantDetail: `connectDomains entry "http://api.other.com"`},
			{name: "Semicolon", csp: `{"resourceDomains":["https://api.other.com;script-src"]}`, wantDetail: `resourceDomains entry "https://api.other.com;script-src"`},
			{name: "Keyword", csp: `{"frameDomains":["'unsafe-eval'"]}`, wantDetail: `frameDomains entry "'unsafe-eval'"`},
			{name: "Dashboard", csp: `{"baseUriDomains":["https://dashboard.test.coder.com"]}`, wantDetail: `baseUriDomains entry "https://dashboard.test.coder.com"`},
			{name: "WorkspaceApp", csp: `{"connectDomains":["https://app--ws--user.apps.test.coder.com"]}`, wantDetail: `connectDomains entry "https://app--ws--user.apps.test.coder.com"`},
			{name: "WildcardOverDashboard", csp: `{"connectDomains":["https://*.test.coder.com"]}`, wantDetail: `connectDomains entry "https://*.test.coder.com"`},
			{name: "IP", csp: `{"connectDomains":["https://127.0.0.1"]}`, wantDetail: `connectDomains entry "https://127.0.0.1"`},
			{name: "Localhost", csp: `{"connectDomains":["wss://localhost:3000"]}`, wantDetail: `connectDomains entry "wss://localhost:3000"`},
			{name: "TooMany", csp: string(tooManyJSON), wantDetail: "connectDomains has 33 entries"},
			{name: "TooLong", csp: string(tooLongJSON), wantDetail: "exceeds"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				srv := newMCPAppSandboxTestServer(t, hostnamePattern, dashboardURL)
				rec, nextCalled := doMCPAppSandboxRequest(t, srv, http.MethodGet, mcpAppSandboxTestHost, "/?csp="+url.QueryEscape(tc.csp))
				require.False(t, nextCalled)
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.Empty(t, rec.Header().Get("Content-Security-Policy"))
				resp := decodeResponse(t, rec)
				require.Equal(t, "Invalid csp query parameter.", resp.Message)
				require.Contains(t, resp.Detail, tc.wantDetail)
			})
		}
	})

	t.Run("AccessURLDiffersFromDashboard", func(t *testing.T) {
		t.Parallel()

		hostnameRegex, err := appurl.CompileHostnamePattern(hostnamePattern)
		require.NoError(t, err)
		dashboard, err := url.Parse(dashboardURL)
		require.NoError(t, err)
		access, err := url.Parse("https://proxy.test.coder.org")
		require.NoError(t, err)
		srv := workspaceapps.NewServer(workspaceapps.ServerOptions{
			Logger:              testutil.Logger(t),
			DashboardURL:        dashboard,
			AccessURL:           access,
			Hostname:            hostnamePattern,
			HostnameRegex:       hostnameRegex,
			SignedTokenProvider: &fakeSignedTokenProvider{},
		})

		for _, entry := range []string{"https://proxy.test.coder.org", "wss://*.test.coder.org"} {
			csp := fmt.Sprintf(`{"connectDomains":[%q]}`, entry)
			rec, nextCalled := doMCPAppSandboxRequest(t, srv, http.MethodGet, mcpAppSandboxTestHost, "/?csp="+url.QueryEscape(csp))
			require.False(t, nextCalled, entry)
			require.Equal(t, http.StatusBadRequest, rec.Code, entry)
			resp := decodeResponse(t, rec)
			require.Contains(t, resp.Detail, fmt.Sprintf("connectDomains entry %q", entry))
		}

		// The meta tag and frame-ancestors still come from the dashboard URL.
		rec, _ := doMCPAppSandboxRequest(t, srv, http.MethodGet, mcpAppSandboxTestHost, "/")
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors https://dashboard.test.coder.com")
		require.Contains(t, rec.Body.String(), `content="https://dashboard.test.coder.com"`)
	})

	t.Run("ValidCSP", func(t *testing.T) {
		t.Parallel()

		csp := `{"connectDomains":["https://api.other.com","wss://ws.other.com:8443"],"resourceDomains":["https://*.cdn.other.com"],"frameDomains":["https://frames.other.com"],"baseUriDomains":["https://base.other.com"],"unknownField":true}`
		srv := newMCPAppSandboxTestServer(t, hostnamePattern, dashboardURL)
		rec, nextCalled := doMCPAppSandboxRequest(t, srv, http.MethodGet, mcpAppSandboxTestHost, "/?csp="+url.QueryEscape(csp))
		require.False(t, nextCalled)
		require.Equal(t, http.StatusOK, rec.Code)

		header := rec.Header().Get("Content-Security-Policy")
		require.Equal(t, strings.Join([]string{
			"default-src 'none'",
			"script-src 'self' 'unsafe-inline' https://*.cdn.other.com",
			"style-src 'self' 'unsafe-inline' https://*.cdn.other.com",
			"connect-src 'self' https://api.other.com wss://ws.other.com:8443",
			"img-src 'self' data: https://*.cdn.other.com",
			"font-src 'self' https://*.cdn.other.com",
			"media-src 'self' data: https://*.cdn.other.com",
			"frame-src https://frames.other.com",
			"object-src 'none'",
			"base-uri https://base.other.com",
			"form-action 'none'",
			"frame-ancestors https://dashboard.test.coder.com",
		}, "; "), header)
	})

	t.Run("EmptyCSP", func(t *testing.T) {
		t.Parallel()

		srv := newMCPAppSandboxTestServer(t, hostnamePattern, dashboardURL)
		for _, query := range []string{"?csp=", "?csp=%7B%7D", "?csp=%7B%22connectDomains%22%3A%5B%5D%7D"} {
			rec, _ := doMCPAppSandboxRequest(t, srv, http.MethodGet, mcpAppSandboxTestHost, "/"+query)
			require.Equal(t, http.StatusOK, rec.Code, query)
			require.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-src 'none'", query)
		}
	})

	t.Run("HostQueryIgnored", func(t *testing.T) {
		t.Parallel()

		srv := newMCPAppSandboxTestServer(t, hostnamePattern, dashboardURL)
		rec, _ := doMCPAppSandboxRequest(t, srv, http.MethodGet, mcpAppSandboxTestHost, "/?host="+url.QueryEscape("https://evil.example.com"))
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		require.Contains(t, body, `<meta name="mcp-app-host-origin" content="https://dashboard.test.coder.com" />`)
		require.NotContains(t, body, "evil.example.com")
		require.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors https://dashboard.test.coder.com")
	})

	t.Run("NonMatchingSubdomainFallsThrough", func(t *testing.T) {
		t.Parallel()

		srv := newMCPAppSandboxTestServer(t, hostnamePattern, dashboardURL)
		// "mcp-xyz" is not a reserved subdomain, so it is parsed as a
		// workspace app URL. It has no separators, so it is parsed as a
		// single-segment app URL, which fails and renders the invalid app
		// URL page instead of the sandbox document.
		rec, nextCalled := doMCPAppSandboxRequest(t, srv, http.MethodGet, "mcp-xyz.apps.test.coder.com", "/")
		require.False(t, nextCalled)
		require.NotEqual(t, http.StatusOK, rec.Code)
		require.Empty(t, rec.Header().Get("Content-Security-Policy"))
		require.NotContains(t, rec.Body.String(), "mcp-app-host-origin")

		// A subdomain that is a valid app URL continues into the normal
		// workspace app flow (which redirects to login with the fake token
		// provider) rather than serving the sandbox document.
		rec, nextCalled = doMCPAppSandboxRequest(t, srv, http.MethodGet, "mcp-0123456789abcdef--ws--user.apps.test.coder.com", "/")
		require.False(t, nextCalled)
		require.Empty(t, rec.Header().Get("Content-Security-Policy"))
		require.NotContains(t, rec.Body.String(), "mcp-app-host-origin")
	})

	t.Run("OtherHostFallsThrough", func(t *testing.T) {
		t.Parallel()

		srv := newMCPAppSandboxTestServer(t, hostnamePattern, dashboardURL)
		rec, nextCalled := doMCPAppSandboxRequest(t, srv, http.MethodGet, "mcp-0123456789abcdef.other.example.com", "/")
		require.True(t, nextCalled)
		require.Empty(t, rec.Header().Get("Content-Security-Policy"))
	})
}
