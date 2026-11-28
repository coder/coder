package workspaceapps_test

// App tests can be found in the apptest package.

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/jwtutils"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/testutil"
)

type fakeSignedTokenProvider struct {
	token *workspaceapps.SignedToken
	// issue writes the error page Issue answers with.
	issue            func(rw http.ResponseWriter)
	fromRequestCalls int
	issueCalls       int
}

func (s *fakeSignedTokenProvider) FromRequest(_ *http.Request) (*workspaceapps.SignedToken, bool) {
	s.fromRequestCalls++
	return s.token, s.token != nil
}

func (s *fakeSignedTokenProvider) Issue(_ context.Context, rw http.ResponseWriter, _ *http.Request, _ workspaceapps.IssueTokenRequest) (*workspaceapps.SignedToken, string, bool) {
	s.issueCalls++
	if s.issue != nil {
		s.issue(rw)
	}
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
}

// Through the real server, an app whose slug is "preview" keeps its hostnames
// with the experiment off or on, and its requests are not taken for a
// preview's, while a request from a preview host is still refused.
func TestHandleSubdomainAppNamedPreview(t *testing.T) {
	t.Parallel()

	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("Experiment=%t", enabled), func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)

			upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
				_, _ = rw.Write([]byte("app ok"))
			}))
			t.Cleanup(upstream.Close)

			dv := coderdtest.DeploymentValues(t)
			dv.HTTPCookies.EnableHostPrefix = true
			if enabled {
				dv.Experiments = []string{string(codersdk.ExperimentOpenPreview)}
			}
			client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
				AppHostname:      "*.test.coder.com",
				DeploymentValues: dv,
			})
			first := coderdtest.CreateFirstUser(t, client)
			me, err := client.User(ctx, codersdk.Me)
			require.NoError(t, err)
			r := dbfake.WorkspaceBuild(t, api.Database, database.WorkspaceTable{
				Name:           "site",
				OwnerID:        me.ID,
				OrganizationID: first.OrganizationID,
			}).WithAgent(func(agents []*proto.Agent) []*proto.Agent {
				agents[0].Apps = []*proto.App{{Slug: "preview", Url: upstream.URL, Subdomain: true}}
				return agents
			}).Do()
			_ = agenttest.New(t, client.URL, r.AgentToken)
			coderdtest.NewWorkspaceAgentWaiter(t, client, r.Workspace.ID).Wait()

			appHost := "preview--dev--site--" + me.Username + ".test.coder.com"
			agentlessAppHost := "preview--site--" + me.Username + ".test.coder.com"
			previewHost := "preview--0123abcd--dev--site--" + me.Username + ".test.coder.com"
			pathApp := "/@" + me.Username + "/site.dev/apps/preview/"
			noRedirects := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			}}
			for _, tc := range []struct {
				name       string
				host       string
				path       string
				origin     string
				wantStatus int
			}{
				{name: "App", host: appHost, path: "/", wantStatus: http.StatusOK},
				{name: "AgentlessApp", host: agentlessAppHost, path: "/", wantStatus: http.StatusOK},
				{name: "AppFromApp", host: agentlessAppHost, path: "/", origin: appHost, wantStatus: http.StatusOK},
				{name: "AppFromPreview", host: appHost, path: "/", origin: previewHost, wantStatus: http.StatusForbidden},
				{name: "PathAppFromApp", path: pathApp, origin: appHost, wantStatus: http.StatusOK},
				{name: "PathAppFromPreview", path: pathApp, origin: previewHost, wantStatus: http.StatusForbidden},
			} {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.URL.String()+tc.path, nil)
				require.NoError(t, err)
				if tc.host != "" {
					req.Host = tc.host
				}
				req.Header.Set(codersdk.SessionTokenHeader, client.SessionToken())
				if tc.origin != "" {
					req.Header.Set(httpmw.OriginHeader, "http://"+tc.origin)
				}
				res, err := noRedirects.Do(req)
				require.NoError(t, err)
				body, err := io.ReadAll(res.Body)
				_ = res.Body.Close()
				require.NoError(t, err)
				require.Equal(t, tc.wantStatus, res.StatusCode, "%s: %s", tc.name, body)
				if tc.wantStatus == http.StatusOK {
					require.Equal(t, "app ok", string(body), tc.name)
				}
			}
		})
	}
}

// A preview host's signed-out page asks the dashboard that frames it to sign
// it in, and the preview CSP allows that inline script.
func TestPreviewSignedOutPage(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentOpenPreview)}
	dv.HTTPCookies.EnableHostPrefix = true
	client := coderdtest.New(t, &coderdtest.Options{
		AppHostname:      "*.test.coder.com",
		DeploymentValues: dv,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.URL.String()+"/", nil)
	require.NoError(t, err)
	req.Host = "preview--8080--dev--site--alice.test.coder.com"
	req.Header.Set("Sec-Fetch-Dest", "iframe")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	require.NoError(t, err)

	require.Equal(t, http.StatusUnauthorized, res.StatusCode, string(body))
	csp := res.Header.Values("Content-Security-Policy")
	require.Len(t, csp, 1)
	require.Contains(t, csp[0], "'unsafe-inline'")
	dashboardOrigin := strings.TrimSuffix(client.URL.String(), "/")
	require.Contains(t, string(body), `<script>window.top.postMessage("coder-preview-sign-in-needed", "`+dashboardOrigin+`")</script>`)
}

// Without --host-prefix-cookie, coderd starts, warns, and its preview hosts
// serve nothing.
func TestPreviewsNeedHostPrefixCookie(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)

	sink := testutil.NewFakeSink(t)
	logger := sink.Logger(slog.LevelWarn)
	dv := coderdtest.DeploymentValues(t)
	dv.Experiments = []string{string(codersdk.ExperimentOpenPreview)}
	client := coderdtest.New(t, &coderdtest.Options{
		AppHostname:      "*.test.coder.com",
		DeploymentValues: dv,
		Logger:           &logger,
	})
	require.Len(t, sink.Entries(func(e slog.SinkEntry) bool {
		return e.Level == slog.LevelWarn && e.Message == "workspace previews are unavailable"
	}), 1)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.URL.String()+"/", nil)
	require.NoError(t, err)
	req.Host = "preview--8080--dev--site--alice.test.coder.com"
	req.Header.Set("Sec-Fetch-Dest", "iframe")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	require.NoError(t, err)

	require.Equal(t, http.StatusNotFound, res.StatusCode, string(body))
	require.Contains(t, string(body), "This server does not serve previews.")
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
