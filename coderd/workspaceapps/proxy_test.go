package workspaceapps_test

// App tests can be found in the apptest package.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

type fakeSignedTokenProvider struct {
	token            *workspaceapps.SignedToken
	fromRequestCalls int
	issueCalls       int
}

func (s *fakeSignedTokenProvider) FromRequest(_ *http.Request) (*workspaceapps.SignedToken, bool) {
	s.fromRequestCalls++
	return s.token, s.token != nil
}

func (s *fakeSignedTokenProvider) Issue(_ context.Context, _ http.ResponseWriter, _ *http.Request, _ workspaceapps.IssueTokenRequest) (*workspaceapps.SignedToken, string, bool) {
	s.issueCalls++
	return nil, "", false
}

func TestHandleSubdomain(t *testing.T) {
	t.Parallel()

	t.Run("IgnoresUntrustedForwardedHost", func(t *testing.T) {
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
	})

	// After consuming a smuggled API key, the handler redirects to strip the key.
	// A path that smuggles a separate host (e.g. "//evil.com") must redirect back
	// to the current origin, not off-site. The full path-sanitization matrix lives
	// in Test_originLocalURL.
	t.Run("APIKeySmugglingStaysOnOrigin", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)

		hostnamePattern := "*--apps.test.coder.com"
		hostnameRegex, err := appurl.CompileHostnamePattern(hostnamePattern)
		require.NoError(t, err)

		dashboardURL, err := url.Parse("https://dashboard.test.coder.com")
		require.NoError(t, err)

		// StaticKey lets us mint a smuggled key the handler can decrypt.
		// A256GCMKW needs a 32-byte key.
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

		// Set r.URL.Path directly: the slash-collapsing middleware writes to chi's
		// RoutePath, not r.URL.Path, so the raw "//evil.com" survives to the handler.
		// "x=1" rides along to confirm unrelated params are preserved.
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

		// The key is consumed and a redirect is issued instead of proxying.
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
	})

	t.Run("FilePreview", func(t *testing.T) {
		t.Parallel()

		filePreviewURL := fmt.Sprintf("http://127.0.0.1:%d/api/v0/previews/0123abcd", workspacesdk.AgentHTTPAPIServerPort)
		for _, tc := range []struct {
			name       string
			subdomain  string
			appURL     string
			path       string
			header     http.Header
			wantID     string
			wantStatus int
		}{
			{
				name:       "Asset",
				subdomain:  "preview--0123abcd--agent--workspace--user",
				appURL:     filePreviewURL,
				path:       "/assets/app.js",
				wantID:     "0123abcd",
				wantStatus: http.StatusNoContent,
			},
			{
				name:      "DropsChatHeaders",
				subdomain: "preview--0123abcd--agent--workspace--user",
				appURL:    filePreviewURL,
				path:      "/",
				header: http.Header{
					workspacesdk.CoderChatIDHeader:          {uuid.NewString()},
					workspacesdk.CoderToolCallIDHeader:      {uuid.NewString()},
					workspacesdk.CoderAncestorChatIDsHeader: {uuid.NewString()},
				},
				wantID:     "0123abcd",
				wantStatus: http.StatusNoContent,
			},
			{
				name:       "DotDot",
				subdomain:  "preview--0123abcd--agent--workspace--user",
				appURL:     filePreviewURL,
				path:       "/../../read-file",
				wantID:     "0123abcd",
				wantStatus: http.StatusNoContent,
			},
			{
				name:       "EncodedDotDot",
				subdomain:  "preview--0123abcd--agent--workspace--user",
				appURL:     filePreviewURL,
				path:       "/%2e%2e/%2e%2e/read-file",
				wantID:     "0123abcd",
				wantStatus: http.StatusNoContent,
			},
			{
				name:       "EncodedSlash",
				subdomain:  "preview--0123abcd--agent--workspace--user",
				appURL:     filePreviewURL,
				path:       "/a%2f..%2f..%2f..%2fread-file",
				wantID:     "0123abcd",
				wantStatus: http.StatusNoContent,
			},
			{
				name:       "Backslash",
				subdomain:  "preview--0123abcd--agent--workspace--user",
				appURL:     filePreviewURL,
				path:       "/..%5C..%5Cread-file",
				wantID:     "0123abcd",
				wantStatus: http.StatusNoContent,
			},
			{
				name:       "PortOnAgentAPI",
				subdomain:  fmt.Sprintf("%d--agent--workspace--user", workspacesdk.AgentHTTPAPIServerPort),
				appURL:     filePreviewURL,
				path:       "/",
				wantStatus: http.StatusBadRequest,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				gotID := make(chan string, 1)
				previews := chi.NewRouter()
				previews.Get("/{id}/*", func(rw http.ResponseWriter, r *http.Request) {
					gotID <- chi.URLParam(r, "id")
					if r.Header.Get(workspacesdk.CoderChatIDHeader) != "" ||
						r.Header.Get(workspacesdk.CoderToolCallIDHeader) != "" ||
						r.Header.Get(workspacesdk.CoderAncestorChatIDsHeader) != "" {
						rw.WriteHeader(http.StatusBadRequest)
						return
					}
					rw.WriteHeader(http.StatusNoContent)
				})
				agentAPI := chi.NewRouter()
				agentAPI.Mount("/api/v0/previews", previews)
				agentAPI.NotFound(func(rw http.ResponseWriter, _ *http.Request) {
					gotID <- ""
					rw.WriteHeader(http.StatusTeapot)
				})
				upstream := httptest.NewServer(agentAPI)
				t.Cleanup(upstream.Close)
				upstreamURL, err := url.Parse(upstream.URL)
				require.NoError(t, err)

				hostnamePattern := "*--apps.test.coder.com"
				hostnameRegex, err := appurl.CompileHostnamePattern(hostnamePattern)
				require.NoError(t, err)
				dashboardURL, err := url.Parse("https://dashboard.test.coder.com")
				require.NoError(t, err)
				app, err := appurl.ParseSubdomainAppURL(tc.subdomain)
				require.NoError(t, err)
				srv := workspaceapps.NewServer(workspaceapps.ServerOptions{
					Logger:        testutil.Logger(t),
					DashboardURL:  dashboardURL,
					AccessURL:     dashboardURL,
					Hostname:      hostnamePattern,
					HostnameRegex: hostnameRegex,
					RealIPConfig:  &httpmw.RealIPConfig{},
					SignedTokenProvider: &fakeSignedTokenProvider{token: &workspaceapps.SignedToken{
						Request: workspaceapps.Request{
							AccessMethod:      workspaceapps.AccessMethodSubdomain,
							BasePath:          "/",
							UsernameOrID:      app.Username,
							WorkspaceNameOrID: app.WorkspaceName,
							AgentNameOrID:     app.AgentName,
							AppSlugOrPort:     app.AppSlugOrPort,
						},
						AppURL: tc.appURL,
					}},
					AgentProvider: &fakeAgentProvider{upstream: upstreamURL},
				})

				host := tc.subdomain + "--apps.test.coder.com"
				req := httptest.NewRequest(http.MethodGet, "https://"+host+tc.path, nil)
				req.Host = host
				for key, values := range tc.header {
					req.Header[key] = values
				}
				rec := httptest.NewRecorder()
				httpmw.ExtractRealIP(srv.RealIPConfig)(srv.HandleSubdomain()(http.NotFoundHandler())).ServeHTTP(rec, req)

				require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
				if tc.wantID == "" {
					require.Empty(t, gotID)
					return
				}
				require.Equal(t, tc.wantID, <-gotID)
			})
		}
	})
}

type fakeAgentProvider struct {
	workspaceapps.AgentProvider
	upstream *url.URL
}

func (p *fakeAgentProvider) ReverseProxy(targetURL, _ *url.URL, _ uuid.UUID, _ appurl.ApplicationURL, _ string) *httputil.ReverseProxy {
	target := *targetURL
	target.Host = p.upstream.Host
	return httputil.NewSingleHostReverseProxy(&target)
}
