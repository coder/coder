package workspaceapps_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/textproto"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/coderd/workspaceapps/previewhost"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
)

const (
	previewDashboard = "https://dashboard.test.coder.com"
	previewPattern   = "*--apps.test.coder.com"
	previewHost      = "preview--8080--agent--workspace--user--apps.test.coder.com"
	previewCSP       = "sandbox allow-scripts allow-same-origin allow-forms allow-modals allow-downloads; " +
		"default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; frame-src 'self'; " +
		"object-src 'none'; form-action 'self'; frame-ancestors 'self' " + previewDashboard
)

func previewUpstream(t *testing.T) *url.URL {
	t.Helper()

	r := chi.NewRouter()
	r.Get("/", func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Add("Set-Cookie", "host=1; Path=/")
		rw.Header().Add("Set-Cookie", "wide=1; Path=/; Domain=test.coder.com")
		rw.Header().Set("Clear-Site-Data", `"cookies"`)
		rw.Header().Set("Link", "<https://other.example.com/x.js>; rel=preload; as=script")
		rw.Header().Set("Access-Control-Allow-Origin", "*")
		_, _ = rw.Write([]byte("preview"))
	})
	r.Get("/early-hints", func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Link", "<https://other.example.com/x.js>; rel=preload; as=script")
		rw.WriteHeader(http.StatusEarlyHints)
		rw.Header().Del("Link")
		_, _ = rw.Write([]byte("preview"))
	})
	r.Post("/repost", func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, "https://app--workspace--user--apps.test.coder.com/", http.StatusTemporaryRedirect)
	})
	r.Get("/ws", func(rw http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(rw, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		typ, msg, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		_ = conn.Write(r.Context(), typ, msg)
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	return u
}

func newPreviewServer(t *testing.T, provider workspaceapps.SignedTokenProvider, upstream *url.URL) *httptest.Server {
	t.Helper()

	dashboardURL, err := url.Parse(previewDashboard)
	require.NoError(t, err)
	hostnameRegex, err := appurl.CompileHostnamePattern(previewPattern)
	require.NoError(t, err)
	hosting, err := previewhost.Strict(previewPattern, dashboardURL, dashboardURL)
	require.NoError(t, err)
	srv := workspaceapps.NewServer(workspaceapps.ServerOptions{
		Logger:              testutil.Logger(t),
		DashboardURL:        dashboardURL,
		AccessURL:           dashboardURL,
		Hostname:            previewPattern,
		HostnameRegex:       hostnameRegex,
		Previews:            hosting,
		RealIPConfig:        &httpmw.RealIPConfig{},
		SignedTokenProvider: provider,
		AgentProvider:       &fakeAgentProvider{upstream: upstream},
	})
	ts := httptest.NewServer(httpmw.ExtractRealIP(srv.RealIPConfig)(srv.HandleSubdomain()(http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	return ts
}

// previewToken returns a signed token for the port preview on host, or nil
// when host does not parse.
func previewToken(host string) *workspaceapps.SignedToken {
	app, err := appurl.ParseSubdomainAppURL(strings.TrimSuffix(host, "--apps.test.coder.com"))
	if err != nil {
		return nil
	}
	port := strings.TrimPrefix(app.AppSlugOrPort, "preview--")
	return &workspaceapps.SignedToken{
		Request: workspaceapps.Request{
			AccessMethod:      workspaceapps.AccessMethodSubdomain,
			BasePath:          "/",
			UsernameOrID:      app.Username,
			WorkspaceNameOrID: app.WorkspaceName,
			AgentNameOrID:     app.AgentName,
			AppSlugOrPort:     app.AppSlugOrPort,
		},
		AppURL: "http://127.0.0.1:" + port,
	}
}

// Every response on a preview host carries the preview CSP, whether coderd
// wrote it or the preview server did.
func TestServePreviewPolicy(t *testing.T) {
	t.Parallel()

	upstream := previewUpstream(t)
	for _, tc := range []struct {
		name         string
		host         string
		method       string
		path         string
		header       http.Header
		signedOut    bool
		deadUpstream bool
		wantStatus   int
		wantLocation string
	}{
		{name: "Proxied", wantStatus: http.StatusOK},
		{name: "ProxiedAfterEarlyHints", path: "/early-hints", wantStatus: http.StatusOK},
		{name: "ServiceWorker", header: http.Header{"Service-Worker": {"script"}}, wantStatus: http.StatusForbidden},
		{
			name:       "FromOtherPreview",
			header:     http.Header{"Origin": {"https://preview--3000--agent--workspace--user--apps.test.coder.com"}},
			wantStatus: http.StatusForbidden,
		},
		{name: "UnparsableHost", host: "preview--08080--agent--workspace--user--apps.test.coder.com", wantStatus: http.StatusBadRequest},
		{name: "PrefixedHost", host: "x---" + previewHost, wantStatus: http.StatusBadRequest},
		{
			name:       "DirectOpen",
			path:       "/docs/?page=2&" + previewhost.SignInParam + "=secret",
			header:     http.Header{"Sec-Fetch-Dest": {"document"}},
			wantStatus: http.StatusSeeOther,
			wantLocation: previewDashboard + "/previews/open?" + url.Values{
				"owner":     {"user"},
				"workspace": {"workspace"},
				"agent":     {"agent"},
				"slug":      {"preview--8080"},
				"path":      {"/docs/?page=2"},
			}.Encode(),
		},
		{name: "NoFetchMetadata", header: http.Header{"Sec-Fetch-Dest": nil}, wantStatus: http.StatusSeeOther},
		{name: "SmuggledAPIKey", path: "/?" + workspaceapps.SubdomainProxyAPIKeyParam + "=x", wantStatus: http.StatusBadRequest},
		{name: "SignedOut", signedOut: true, wantStatus: http.StatusUnauthorized},
		{name: "AgentUnreachable", deadUpstream: true, wantStatus: http.StatusBadGateway},
		{name: "RepostToOtherHost", method: http.MethodPost, path: "/repost", wantStatus: http.StatusForbidden},
		{name: "ReservedPort", host: "preview--4--agent--workspace--user--apps.test.coder.com", wantStatus: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)

			host := previewHost
			if tc.host != "" {
				host = tc.host
			}
			provider := &fakeSignedTokenProvider{token: previewToken(host)}
			if tc.signedOut {
				// Workspace proxies copy coderd's error page into the map.
				provider = &fakeSignedTokenProvider{issue: func(rw http.ResponseWriter) {
					rw.Header()["Content-Type"] = []string{"text/html; charset=utf-8"}
					rw.WriteHeader(http.StatusUnauthorized)
				}}
			}
			target := upstream
			if tc.deadUpstream {
				target = &url.URL{Scheme: "http", Host: "127.0.0.1:1"}
			}
			ts := newPreviewServer(t, provider, target)

			method := tc.method
			if method == "" {
				method = http.MethodGet
			}
			var interim []int
			trace := &httptrace.ClientTrace{Got1xxResponse: func(code int, _ textproto.MIMEHeader) error {
				interim = append(interim, code)
				return nil
			}}
			req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), method, ts.URL+tc.path, nil)
			require.NoError(t, err)
			req.Host = host
			req.Header.Set("Sec-Fetch-Dest", "iframe")
			for key, values := range tc.header {
				req.Header[key] = values
			}
			client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			}}
			res, err := client.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			body, _ := io.ReadAll(res.Body)
			require.Equal(t, tc.wantStatus, res.StatusCode, string(body))
			require.Equal(t, []string{previewCSP}, res.Header.Values("Content-Security-Policy"))
			require.Empty(t, interim, "interim responses must not reach the browser")
			require.Empty(t, res.Header.Values("Link"))
			if tc.wantLocation != "" {
				require.Equal(t, tc.wantLocation, res.Header.Get("Location"))
			}
			if tc.name == "Proxied" {
				require.Equal(t, []string{"host=1; Path=/"}, res.Header.Values("Set-Cookie"))
				require.Empty(t, res.Header.Get("Clear-Site-Data"))
				require.Empty(t, res.Header.Get("Access-Control-Allow-Origin"))
			}
		})
	}

	t.Run("WebSocket", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)

		ts := newPreviewServer(t, &fakeSignedTokenProvider{token: previewToken(previewHost)}, upstream)
		// The response to a successful dial has no body.
		// nolint:bodyclose
		conn, res, err := websocket.Dial(ctx, strings.Replace(ts.URL, "http", "ws", 1)+"/ws", &websocket.DialOptions{
			Host:       previewHost,
			HTTPHeader: http.Header{"Sec-Fetch-Dest": {"websocket"}},
		})
		require.NoError(t, err)
		defer conn.CloseNow()
		require.Equal(t, http.StatusSwitchingProtocols, res.StatusCode)

		require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte("hello")))
		_, msg, err := conn.Read(ctx)
		require.NoError(t, err)
		require.Equal(t, "hello", string(msg))
	})
}

// Requests from a preview page to the user's other apps are refused before
// they act as the user, and a server without previews never serves one.
func TestHandleSubdomainPreviewIsolation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	dashboardURL, err := url.Parse(previewDashboard)
	require.NoError(t, err)
	hostnameRegex, err := appurl.CompileHostnamePattern(previewPattern)
	require.NoError(t, err)
	provider := &fakeSignedTokenProvider{}
	srv := workspaceapps.NewServer(workspaceapps.ServerOptions{
		Logger:              testutil.Logger(t),
		DashboardURL:        dashboardURL,
		AccessURL:           dashboardURL,
		Hostname:            previewPattern,
		HostnameRegex:       hostnameRegex,
		RealIPConfig:        &httpmw.RealIPConfig{},
		SignedTokenProvider: provider,
	})

	serve := func(host string, header http.Header) int {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "https://"+host+"/", nil)
		req.Host = host
		for key, values := range header {
			req.Header[key] = values
		}
		rec := httptest.NewRecorder()
		srv.HandleSubdomain()(http.NotFoundHandler()).ServeHTTP(rec, req)
		return rec.Code
	}

	fromPreview := http.Header{"Origin": {"https://" + previewHost}}
	require.Equal(t, http.StatusForbidden, serve("app--workspace--user--apps.test.coder.com", fromPreview))
	require.Equal(t, http.StatusNotFound, serve(previewHost, http.Header{"Sec-Fetch-Dest": {"iframe"}}))
	require.Zero(t, provider.issueCalls)
	require.Zero(t, provider.fromRequestCalls)

	// A request from a non-preview origin still reaches the app's own auth.
	serve("app--workspace--user--apps.test.coder.com", http.Header{"Origin": {"https://other--workspace--user--apps.test.coder.com"}})
	require.Equal(t, 1, provider.issueCalls)
}
