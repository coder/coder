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

	// Previews serve agent-authored content. Only file previews may reach the
	// agent's reserved preview port, preview responses are sandboxed, and
	// preview pages cannot send requests to other apps.
	t.Run("Previews", func(t *testing.T) {
		t.Parallel()

		const (
			previewHost = "preview--0123456789abcdef--agent--workspace--user"
			portHost    = "8080--agent--workspace--user"
		)
		previewPortURL := fmt.Sprintf("http://127.0.0.1:%d", workspacesdk.AgentPreviewPort)
		apiPortURL := fmt.Sprintf("http://127.0.0.1:%d", workspacesdk.AgentHTTPAPIServerPort)
		cases := []struct {
			name      string
			subdomain string
			appURL    string
			origin    string
			method    string
			header    http.Header
			location  string
			// redirectStatus is the upstream status when location is set.
			redirectStatus int
			wantStatus     int
			wantBody       string
			wantPath       string
			wantCSP        bool
		}{
			{
				name:       "FilePreview",
				subdomain:  previewHost,
				appURL:     previewPortURL + "/0123456789abcdef",
				wantStatus: http.StatusNoContent,
				wantPath:   "/0123456789abcdef/assets/app.js",
				wantCSP:    true,
			},
			{
				name:       "PortPreview",
				subdomain:  "preview--8080--agent--workspace--user",
				appURL:     "http://127.0.0.1:8080",
				wantStatus: http.StatusNoContent,
				wantPath:   "/assets/app.js",
				wantCSP:    true,
			},
			{
				// Browsers repeat a POST at a 307 or 308 target with a null
				// Origin, which the Origin check cannot attribute.
				name:           "PortPreviewRepostToOtherHost",
				subdomain:      "preview--8080--agent--workspace--user",
				appURL:         "http://127.0.0.1:8080",
				method:         http.MethodPost,
				location:       "https://" + portHost + "--apps.test.coder.com/settings",
				redirectStatus: http.StatusTemporaryRedirect,
				wantStatus:     http.StatusBadGateway,
				wantPath:       "/assets/app.js",
			},
			{
				name:           "PortPreviewRepostToSameHost",
				subdomain:      "preview--8080--agent--workspace--user",
				appURL:         "http://127.0.0.1:8080",
				method:         http.MethodPost,
				location:       "/login",
				redirectStatus: http.StatusTemporaryRedirect,
				wantStatus:     http.StatusTemporaryRedirect,
				wantPath:       "/assets/app.js",
				wantCSP:        true,
			},
			{
				// Browsers read backslashes as slashes, so these name another
				// host even though url.Parse finds none.
				name:           "PortPreviewRepostToOtherHostBackslashScheme",
				subdomain:      "preview--8080--agent--workspace--user",
				appURL:         "http://127.0.0.1:8080",
				method:         http.MethodPost,
				location:       `https:\\` + portHost + `--apps.test.coder.com/settings`,
				redirectStatus: http.StatusTemporaryRedirect,
				wantStatus:     http.StatusBadGateway,
				wantPath:       "/assets/app.js",
			},
			{
				name:           "PortPreviewRepostToOtherHostBackslashRelative",
				subdomain:      "preview--8080--agent--workspace--user",
				appURL:         "http://127.0.0.1:8080",
				method:         http.MethodPost,
				location:       `/\` + portHost + `--apps.test.coder.com/settings`,
				redirectStatus: http.StatusTemporaryRedirect,
				wantStatus:     http.StatusBadGateway,
				wantPath:       "/assets/app.js",
			},
			{
				name:           "PortPreviewRepostToOtherHostTripleSlash",
				subdomain:      "preview--8080--agent--workspace--user",
				appURL:         "http://127.0.0.1:8080",
				method:         http.MethodPost,
				location:       "///" + portHost + "--apps.test.coder.com/settings",
				redirectStatus: http.StatusTemporaryRedirect,
				wantStatus:     http.StatusBadGateway,
				wantPath:       "/assets/app.js",
			},
			{
				// With a scheme other than the page's, browsers read a host
				// after a single slash.
				name:           "PortPreviewRepostToOtherHostSingleSlash",
				subdomain:      "preview--8080--agent--workspace--user",
				appURL:         "http://127.0.0.1:8080",
				method:         http.MethodPost,
				location:       "http:/" + portHost + "--apps.test.coder.com/settings",
				redirectStatus: http.StatusTemporaryRedirect,
				wantStatus:     http.StatusBadGateway,
				wantPath:       "/assets/app.js",
			},
			{
				// 302 keeps every method but POST.
				name:           "PortPreviewFoundPutToOtherHost",
				subdomain:      "preview--8080--agent--workspace--user",
				appURL:         "http://127.0.0.1:8080",
				method:         http.MethodPut,
				location:       "https://" + portHost + "--apps.test.coder.com/settings",
				redirectStatus: http.StatusFound,
				wantStatus:     http.StatusBadGateway,
				wantPath:       "/assets/app.js",
			},
			{
				// Browsers follow a 302 after a POST with GET, like a link.
				name:           "PortPreviewFoundPostToOtherHost",
				subdomain:      "preview--8080--agent--workspace--user",
				appURL:         "http://127.0.0.1:8080",
				method:         http.MethodPost,
				location:       "https://" + portHost + "--apps.test.coder.com/settings",
				redirectStatus: http.StatusFound,
				wantStatus:     http.StatusFound,
				wantPath:       "/assets/app.js",
				wantCSP:        true,
			},
			{
				name:           "PortPreviewRepostToSameHostAbsolute",
				subdomain:      "preview--8080--agent--workspace--user",
				appURL:         "http://127.0.0.1:8080",
				method:         http.MethodPost,
				location:       "https://preview--8080--agent--workspace--user--apps.test.coder.com/login",
				redirectStatus: http.StatusTemporaryRedirect,
				wantStatus:     http.StatusTemporaryRedirect,
				wantPath:       "/assets/app.js",
				wantCSP:        true,
			},
			{
				name:       "PortPreviewServiceWorker",
				subdomain:  "preview--8080--agent--workspace--user",
				appURL:     "http://127.0.0.1:8080",
				header:     http.Header{"Service-Worker": {"script"}},
				wantStatus: http.StatusForbidden,
				wantBody:   "Preview pages cannot register service workers.",
			},
			{
				name:       "PortPreviewOnPreviewPort",
				subdomain:  "preview--8080--agent--workspace--user",
				appURL:     previewPortURL,
				wantStatus: http.StatusBadRequest,
				wantBody:   "Coder reserves ports less than",
			},
			{
				name:       "PreviewLikeAppOnPreviewPort",
				subdomain:  "preview-0123456789abcdef--workspace--user",
				appURL:     previewPortURL,
				wantStatus: http.StatusBadRequest,
				wantBody:   "Coder reserves ports less than",
			},
			{
				name:       "FilePreviewOnOtherReservedPort",
				subdomain:  previewHost,
				appURL:     apiPortURL + "/0123456789abcdef",
				wantStatus: http.StatusBadRequest,
				wantBody:   "Coder reserves ports less than",
			},
			{
				name:       "RequestFromSamePreview",
				subdomain:  previewHost,
				appURL:     previewPortURL + "/0123456789abcdef",
				origin:     "https://" + previewHost + "--apps.test.coder.com",
				wantStatus: http.StatusNoContent,
				wantPath:   "/0123456789abcdef/assets/app.js",
				wantCSP:    true,
			},
			{
				name:       "RequestFromOtherPreview",
				subdomain:  portHost,
				appURL:     "http://127.0.0.1:8080",
				origin:     "https://" + previewHost + "--apps.test.coder.com",
				wantStatus: http.StatusForbidden,
				wantBody:   "Preview pages cannot send requests to other apps.",
			},
			{
				name:       "RequestFromPreviewOnOtherProxy",
				subdomain:  portHost,
				appURL:     "http://127.0.0.1:8080",
				origin:     "https://preview--3000--agent--workspace--user.proxy.example.com",
				wantStatus: http.StatusForbidden,
				wantBody:   "Preview pages cannot send requests to other apps.",
			},
			{
				name:       "RequestFromOtherApp",
				subdomain:  portHost,
				appURL:     "http://127.0.0.1:8080",
				origin:     "https://3000--agent--workspace--user--apps.test.coder.com",
				wantStatus: http.StatusNoContent,
				wantPath:   "/assets/app.js",
			},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()

				gotPath := make(chan string, 1)
				upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
					gotPath <- r.URL.Path
					if c.location != "" {
						// http.Redirect would rewrite relative locations.
						rw.Header().Set("Location", c.location)
						rw.WriteHeader(c.redirectStatus)
						return
					}
					rw.WriteHeader(http.StatusNoContent)
				}))
				t.Cleanup(upstream.Close)
				upstreamURL, err := url.Parse(upstream.URL)
				require.NoError(t, err)

				hostnamePattern := "*--apps.test.coder.com"
				hostnameRegex, err := appurl.CompileHostnamePattern(hostnamePattern)
				require.NoError(t, err)
				dashboardURL, err := url.Parse("https://dashboard.test.coder.com")
				require.NoError(t, err)

				app, err := appurl.ParseSubdomainAppURL(c.subdomain)
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
						AppURL: c.appURL,
					}},
					AgentProvider: &fakeAgentProvider{upstream: upstreamURL},
				})

				host := c.subdomain + "--apps.test.coder.com"
				method := c.method
				if method == "" {
					method = http.MethodGet
				}
				req := httptest.NewRequest(method, "https://"+host+"/assets/app.js", nil)
				req.Host = host
				if c.origin != "" {
					req.Header.Set("Origin", c.origin)
				}
				for key, values := range c.header {
					req.Header[key] = values
				}
				rec := httptest.NewRecorder()
				httpmw.ExtractRealIP(srv.RealIPConfig)(srv.HandleSubdomain()(http.NotFoundHandler())).ServeHTTP(rec, req)

				require.Equal(t, c.wantStatus, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), c.wantBody)
				if c.wantCSP {
					require.Equal(t, "sandbox allow-scripts allow-same-origin allow-forms allow-modals allow-downloads; frame-src 'self'; object-src 'none'", rec.Header().Get("Content-Security-Policy"))
				} else {
					require.Empty(t, rec.Header().Get("Content-Security-Policy"))
				}
				if c.wantPath == "" {
					require.Empty(t, gotPath)
					return
				}
				require.Equal(t, c.wantPath, <-gotPath)
			})
		}
	})
}

type fakeAgentProvider struct {
	workspaceapps.AgentProvider
	upstream *url.URL
}

func (p *fakeAgentProvider) ReverseProxy(targetURL, _ *url.URL, _ uuid.UUID, _ appurl.ApplicationURL, _ string) *httputil.ReverseProxy {
	tgt := *targetURL
	tgt.Host = p.upstream.Host
	return httputil.NewSingleHostReverseProxy(&tgt)
}

func TestWorkspaceAppsProxyPathRequestFromPreview(t *testing.T) {
	t.Parallel()

	dashboardURL, err := url.Parse("https://dashboard.test.coder.com")
	require.NoError(t, err)
	srv := workspaceapps.NewServer(workspaceapps.ServerOptions{
		Logger:       testutil.Logger(t),
		DashboardURL: dashboardURL,
		AccessURL:    dashboardURL,
		RealIPConfig: &httpmw.RealIPConfig{},
	})
	router := chi.NewRouter()
	srv.Attach(router)

	req := httptest.NewRequest(http.MethodPost, "https://dashboard.test.coder.com/@user/workspace/apps/app/", nil)
	req.Header.Set("Origin", "https://preview--3000--agent--workspace--user.apps.test.coder.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "Preview pages cannot send requests to other apps.")
}
