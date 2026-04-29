package aibridge //nolint:testpackage // tests unexported newPassthroughRouter

import (
	"crypto/tls"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
)

var testTracer = otel.Tracer("bridge_test")

func TestPassthroughRoutes(t *testing.T) {
	t.Parallel()

	upstreamRespBody := "upstream response"
	tests := []struct {
		name              string
		baseURLPath       string
		reqPath           string
		reqHost           string
		reqRemoteAddr     string
		reqHeaders        http.Header
		expectRequestPath string
		expectQuery       string
		expectHeaders     http.Header
		expectRespStatus  int
		expectRespBody    string
	}{
		{
			name:              "passthrough_route_no_path",
			reqPath:           "/v1/conversations",
			expectRequestPath: "/v1/conversations",
			expectRespStatus:  http.StatusOK,
			expectRespBody:    upstreamRespBody,
		},
		{
			name:              "base_URL_path_is_preserved_in_passthrough_routes",
			baseURLPath:       "/api/v2",
			reqPath:           "/v1/models",
			expectRequestPath: "/api/v2/v1/models",
			expectRespStatus:  http.StatusOK,
			expectRespBody:    upstreamRespBody,
		},
		{
			name:             "passthrough_route_break_parse_base_url",
			baseURLPath:      "/%zz",
			reqPath:          "/v1/models/",
			expectRespStatus: http.StatusBadGateway,
			expectRespBody:   "invalid provider base URL",
		},
		{
			name:             "passthrough_route_rejects_invalid_base_url_path",
			baseURLPath:      "/%25",
			reqPath:          "/v1/models",
			expectRespStatus: http.StatusBadGateway,
			expectRespBody:   "invalid provider base URL",
		},
		{
			name:          "proxy_headers_are_set_and_forwarded_chain_is_appended",
			reqPath:       "/v1/models",
			reqHost:       "client.example.com",
			reqRemoteAddr: "1.1.1.1:1111",
			reqHeaders: http.Header{
				"X-Forwarded-For": {"2.2.2.2, 3.3.3.3"},
			},
			expectRequestPath: "/v1/models",
			expectRespStatus:  http.StatusOK,
			expectRespBody:    upstreamRespBody,
			expectHeaders: http.Header{
				"Accept-Encoding":   {"gzip"},
				"User-Agent":        {"aibridge"},
				"X-Forwarded-For":   {"2.2.2.2, 3.3.3.3, 1.1.1.1"},
				"X-Forwarded-Host":  {"client.example.com"},
				"X-Forwarded-Proto": {"http"},
			},
		},
		{
			name:              "query_string_is_preserved",
			reqPath:           "/v1/models?search=gpt&limit=10",
			expectRequestPath: "/v1/models",
			expectQuery:       "search=gpt&limit=10",
			expectRespStatus:  http.StatusOK,
			expectRespBody:    upstreamRespBody,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := slogtest.Make(t, nil)

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.expectRequestPath, r.URL.Path)
				assert.Equal(t, tc.expectQuery, r.URL.RawQuery)
				if tc.expectHeaders != nil {
					assert.Equal(t, tc.expectHeaders, r.Header)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(upstreamRespBody))
			}))
			t.Cleanup(upstream.Close)

			prov := &testutil.MockProvider{
				URL: upstream.URL + tc.baseURLPath,
			}

			handler := newPassthroughRouter(prov, logger, nil, testTracer)

			req := httptest.NewRequest("", tc.reqPath, nil)
			maps.Copy(req.Header, tc.reqHeaders)
			req.Host = tc.reqHost
			req.RemoteAddr = tc.reqRemoteAddr
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)

			assert.Equal(t, tc.expectRespStatus, resp.Code)
			assert.Contains(t, resp.Body.String(), tc.expectRespBody)
		})
	}
}

func TestRewritePassthroughRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		reqPath       string
		reqRemoteAddr string
		reqHeaders    http.Header
		reqTLS        bool
		provider      *testutil.MockProvider
		expectURL     string
		expectHeaders http.Header
	}{
		{
			name:          "sets_upstream_url_and_forwarded_headers_from_client_peer",
			reqPath:       "http://client-host/chat?stream=true",
			reqRemoteAddr: "1.1.1.1:1111",
			provider:      &testutil.MockProvider{URL: "https://upstream-host/base"},
			expectURL:     "https://upstream-host/base/chat?stream=true",
			expectHeaders: http.Header{
				"X-Forwarded-Host":  {"client-host"},
				"X-Forwarded-Proto": {"http"},
				"X-Forwarded-For":   {"1.1.1.1"},
				"User-Agent":        {"aibridge"},
			},
		},
		{
			name:          "preserves_client_user_agent",
			reqPath:       "http://client-host/chat",
			reqRemoteAddr: "1.1.1.1:1111",
			reqHeaders:    http.Header{"User-Agent": {"custom-agent/1.0"}},
			provider:      &testutil.MockProvider{URL: "https://upstream-host/base"},
			expectURL:     "https://upstream-host/base/chat",
			expectHeaders: http.Header{
				"X-Forwarded-Host":  {"client-host"},
				"X-Forwarded-Proto": {"http"},
				"X-Forwarded-For":   {"1.1.1.1"},
				"User-Agent":        {"custom-agent/1.0"},
			},
		},
		{
			name:          "injects_auth_header",
			reqPath:       "http://client-host/chat",
			reqRemoteAddr: "1.1.1.1:1111",
			provider: &testutil.MockProvider{
				URL: "https://upstream-host/base",
				InjectAuthHeaderFunc: func(h *http.Header) {
					h.Set("Authorization", "Bearer test-token")
				},
			},
			expectURL: "https://upstream-host/base/chat",
			expectHeaders: http.Header{
				"X-Forwarded-Host":  {"client-host"},
				"X-Forwarded-Proto": {"http"},
				"X-Forwarded-For":   {"1.1.1.1"},
				"User-Agent":        {"aibridge"},
				"Authorization":     {"Bearer test-token"},
			},
		},
		{
			name:          "appends_remote_addr_to_existing_forwarded_for_chain",
			reqPath:       "http://client-host/chat",
			reqRemoteAddr: "1.1.1.1:1111",
			reqHeaders: http.Header{
				"X-Forwarded-For": {"2.2.2.2, 3.3.3.3"},
			},
			provider:  &testutil.MockProvider{URL: "https://upstream-host/base"},
			expectURL: "https://upstream-host/base/chat",
			expectHeaders: http.Header{
				"X-Forwarded-Host":  {"client-host"},
				"X-Forwarded-Proto": {"http"},
				"X-Forwarded-For":   {"2.2.2.2, 3.3.3.3, 1.1.1.1"},
				"User-Agent":        {"aibridge"},
			},
		},
		{
			name:          "tls_request_sets_forwarded_proto_to_https",
			reqPath:       "http://client-host/chat",
			reqRemoteAddr: "1.1.1.1:1111",
			reqTLS:        true,
			provider:      &testutil.MockProvider{URL: "https://upstream-host/base"},
			expectURL:     "https://upstream-host/base/chat",
			expectHeaders: http.Header{
				"X-Forwarded-Host":  {"client-host"},
				"X-Forwarded-Proto": {"https"},
				"X-Forwarded-For":   {"1.1.1.1"},
				"User-Agent":        {"aibridge"},
			},
		},
		{
			// This is an edge case where whole `X-Forwarded-For` header
			// is dropped if last hop (remote addr) is not parseable.
			// This is how library handles this case and is not directly
			// related to our code. Added it to verify that we
			// don't accidentally break this behavior.
			name:          "omits_forwarded_for_when_remote_addr_is_not_parseable",
			reqPath:       "http://client-host/chat",
			reqRemoteAddr: "not-a-socket-address",
			reqHeaders: http.Header{
				"X-Forwarded-For": {"1.1.1.1"},
			},
			provider:  &testutil.MockProvider{URL: "https://upstream-host/base"},
			expectURL: "https://upstream-host/base/chat",
			expectHeaders: http.Header{
				"X-Forwarded-Host":  {"client-host"},
				"X-Forwarded-Proto": {"http"},
				"User-Agent":        {"aibridge"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, tc.reqPath, nil)
			maps.Copy(r.Header, tc.reqHeaders)
			r.RemoteAddr = tc.reqRemoteAddr
			if tc.reqTLS {
				r.TLS = &tls.ConnectionState{}
			}
			provBaseURL, err := url.Parse(tc.provider.URL)
			assert.NoError(t, err)

			pr := &httputil.ProxyRequest{
				In:  r,
				Out: r.Clone(r.Context()),
			}

			rewritePassthroughRequest(pr, provBaseURL, tc.provider)

			assert.Equal(t, tc.expectURL, pr.Out.URL.String())
			assert.Equal(t, "", pr.Out.Host)
			assert.Equal(t, tc.expectHeaders, pr.Out.Header)
		})
	}
}

func TestPassthroughRouterReusesProxyInstance(t *testing.T) {
	t.Parallel()

	var newConnections atomic.Int32
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConnections.Add(1)
		}
	}
	upstream.Start()
	t.Cleanup(upstream.Close)

	logger := slogtest.Make(t, nil)
	prov := &testutil.MockProvider{URL: upstream.URL}
	handler := newPassthroughRouter(prov, logger, nil, testTracer)

	for i := range 2 {
		req := httptest.NewRequest(http.MethodGet, "http://proxy.example.test/v1/models", nil)
		resp := httptest.NewRecorder()

		handler.ServeHTTP(resp, req)

		assert.Equalf(t, http.StatusOK, resp.Code, "request %d", i+1)
		assert.Equal(t, "ok", resp.Body.String())
	}

	assert.EqualValues(t, 1, newConnections.Load())
}
