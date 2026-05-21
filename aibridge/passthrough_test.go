package aibridge //nolint:testpackage // tests unexported newPassthroughRouter

import (
	"crypto/tls"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/quartz"
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

			rewritePassthroughRequest(pr, provBaseURL)

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

// TestPassthrough_KeyFailover exercises the KeyFailoverTransport
// end-to-end through the passthrough proxy, parameterised over
// providers (anthropic, openai). Each scenario asserts the upstream
// request count, the response status and Retry-After, and the final
// pool state.
func TestPassthrough_KeyFailover(t *testing.T) {
	t.Parallel()

	type upstreamResponse struct {
		statusCode int
		body       string
		headers    map[string]string
	}

	const (
		rateLimitBody   = `{"error":"rate"}`
		authErrorBody   = `{"error":"unauthorized"}`
		serverErrorBody = `{"error":"server"}`
		successBody     = `{"data":[]}`
	)

	// providers parameterises the table over the two providers
	// that support key failover. Each entry encapsulates the
	// provider-specific bits the test needs: how the mock upstream
	// extracts the key from the request, how a BYOK request sets
	// it, and how the provider is constructed for a given pool.
	providers := []struct {
		name        string
		extractKey  func(*http.Request) string
		setBYOK     func(*http.Request, string)
		newProvider func(baseURL string, pool *keypool.Pool) provider.Provider
		byokOnly    bool
	}{
		{
			name: "anthropic",
			extractKey: func(r *http.Request) string {
				return r.Header.Get("X-Api-Key")
			},
			setBYOK: func(r *http.Request, key string) {
				r.Header.Set("X-Api-Key", key)
			},
			newProvider: func(baseURL string, pool *keypool.Pool) provider.Provider {
				return provider.NewAnthropic(config.Anthropic{
					BaseURL: baseURL,
					KeyPool: pool,
				}, nil)
			},
		},
		{
			name: "openai",
			extractKey: func(r *http.Request) string {
				return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			},
			setBYOK: func(r *http.Request, key string) {
				r.Header.Set("Authorization", "Bearer "+key)
			},
			newProvider: func(baseURL string, pool *keypool.Pool) provider.Provider {
				cfg := config.OpenAI{BaseURL: baseURL}
				if pool != nil {
					cfg.KeyPool = pool
				}
				return provider.NewOpenAI(cfg)
			},
		},
		{
			// Copilot is always BYOK and returns an empty KeyFailoverConfig,
			// which makes the KeyFailoverTransport short-circuit. Only the
			// BYOK scenario applies, centralized-pool cases are skipped below.
			name: "copilot",
			extractKey: func(r *http.Request) string {
				return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			},
			setBYOK: func(r *http.Request, key string) {
				r.Header.Set("Authorization", "Bearer "+key)
			},
			newProvider: func(baseURL string, _ *keypool.Pool) provider.Provider {
				return provider.NewCopilot(config.Copilot{BaseURL: baseURL})
			},
			byokOnly: true,
		},
	}

	tests := []struct {
		name string
		// Centralized pool keys. Empty when byokKey is set.
		keys []string
		// BYOK key. Empty when keys is set.
		byokKey string
		// Scripted upstream responses keyed by API key value.
		responses            map[string]upstreamResponse
		expectedRequestCount int32
		expectedStatusCode   int
		expectedRetryAfter   string
		// Expected key states after the request, by index in keys.
		expectedKeyStates []keypool.KeyState
	}{
		{
			// Given: 1 valid key returning 200.
			// Then: 1 request, 200 response, key remains valid.
			name: "single_valid_key",
			keys: []string{"k0"},
			responses: map[string]upstreamResponse{
				"k0": {statusCode: http.StatusOK, body: successBody},
			},
			expectedRequestCount: 1,
			expectedStatusCode:   http.StatusOK,
			expectedKeyStates:    []keypool.KeyState{keypool.KeyStateValid},
		},
		{
			// Given: 2 keys; key-0 returns 429, key-1 returns 200.
			// Then: 2 requests, 200 response, key-0 temporary, key-1 valid.
			name: "failover_after_429",
			keys: []string{"k0", "k1"},
			responses: map[string]upstreamResponse{
				"k0": {
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "5"},
					body:       rateLimitBody,
				},
				"k1": {statusCode: http.StatusOK, body: successBody},
			},
			expectedRequestCount: 2,
			expectedStatusCode:   http.StatusOK,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateValid,
			},
		},
		{
			// Given: 2 keys; key-0 returns 401, key-1 returns 200.
			// Then: 2 requests, 200 response, key-0 permanent, key-1 valid.
			name: "failover_after_401",
			keys: []string{"k0", "k1"},
			responses: map[string]upstreamResponse{
				"k0": {statusCode: http.StatusUnauthorized, body: authErrorBody},
				"k1": {statusCode: http.StatusOK, body: successBody},
			},
			expectedRequestCount: 2,
			expectedStatusCode:   http.StatusOK,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStatePermanent,
				keypool.KeyStateValid,
			},
		},
		{
			// Given: 2 keys; key-0 returns 403, key-1 returns 200.
			// Then: 2 requests, 200 response, key-0 permanent, key-1 valid.
			name: "failover_after_403",
			keys: []string{"k0", "k1"},
			responses: map[string]upstreamResponse{
				"k0": {statusCode: http.StatusForbidden, body: authErrorBody},
				"k1": {statusCode: http.StatusOK, body: successBody},
			},
			expectedRequestCount: 2,
			expectedStatusCode:   http.StatusOK,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStatePermanent,
				keypool.KeyStateValid,
			},
		},
		{
			// Given: 3 keys; all return 429 with cooldowns 5s, 3s, 10s.
			// Then: 3 requests, 429 response with smallest Retry-After,
			// all keys temporary.
			name: "all_keys_rate_limited",
			keys: []string{"k0", "k1", "k2"},
			responses: map[string]upstreamResponse{
				"k0": {
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "5"},
					body:       rateLimitBody,
				},
				"k1": {
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "3"},
					body:       rateLimitBody,
				},
				"k2": {
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "10"},
					body:       rateLimitBody,
				},
			},
			expectedRequestCount: 3,
			expectedStatusCode:   http.StatusTooManyRequests,
			expectedRetryAfter:   "3",
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateTemporary,
				keypool.KeyStateTemporary,
			},
		},
		{
			// Given: 2 keys; both return 401.
			// Then: 2 requests, 502 response, both keys permanent.
			name: "all_keys_unauthorized",
			keys: []string{"k0", "k1"},
			responses: map[string]upstreamResponse{
				"k0": {statusCode: http.StatusUnauthorized, body: authErrorBody},
				"k1": {statusCode: http.StatusUnauthorized, body: authErrorBody},
			},
			expectedRequestCount: 2,
			expectedStatusCode:   http.StatusBadGateway,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStatePermanent,
				keypool.KeyStatePermanent,
			},
		},
		{
			// Given: 2 keys; key-0 returns 500.
			// Then: 1 request, 500 response, both keys remain valid.
			name: "server_error_no_failover",
			keys: []string{"k0", "k1"},
			responses: map[string]upstreamResponse{
				"k0": {statusCode: http.StatusInternalServerError, body: serverErrorBody},
			},
			expectedRequestCount: 1,
			expectedStatusCode:   http.StatusInternalServerError,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateValid,
				keypool.KeyStateValid,
			},
		},
		{
			// Given: BYOK with a single user-supplied key returning 429.
			// Then: 1 request, 429 forwarded as-is, no failover.
			name:    "byok_no_failover",
			byokKey: "user-byok",
			responses: map[string]upstreamResponse{
				"user-byok": {
					statusCode: http.StatusTooManyRequests,
					headers:    map[string]string{"Retry-After": "5"},
					body:       rateLimitBody,
				},
			},
			expectedRequestCount: 1,
			expectedStatusCode:   http.StatusTooManyRequests,
			expectedRetryAfter:   "5",
		},
	}

	for _, prov := range providers {
		for _, tc := range tests {
			// BYOK-only providers do not exercise centralized-pool scenarios.
			if prov.byokOnly && tc.byokKey == "" {
				continue
			}
			t.Run(prov.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()

				// Mock upstream: counts requests and returns
				// scripted responses keyed by API key. An unmapped
				// key falls through to 500 so misconfigured cases
				// surface via the status assertion.
				var requestCount atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requestCount.Add(1)
					_, _ = io.Copy(io.Discard, r.Body)
					resp, ok := tc.responses[prov.extractKey(r)]
					if !ok {
						resp = upstreamResponse{statusCode: http.StatusInternalServerError}
					}
					w.Header().Set("Content-Type", "application/json")
					for hk, hv := range resp.headers {
						w.Header().Set(hk, hv)
					}
					w.WriteHeader(resp.statusCode)
					_, _ = w.Write([]byte(resp.body))
				}))
				t.Cleanup(upstream.Close)

				var pool *keypool.Pool
				if len(tc.keys) > 0 {
					var err error
					pool, err = keypool.New(tc.keys, quartz.NewMock(t))
					require.NoError(t, err)
				}

				p := prov.newProvider(upstream.URL, pool)
				// IgnoreErrors: MarkKey logs at ERROR level when a
				// key is marked permanent (401/403); slogtest would
				// otherwise fail those scenarios.
				logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
				handler := newPassthroughRouter(p, logger, nil, testTracer)

				req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
				if tc.byokKey != "" {
					prov.setBYOK(req, tc.byokKey)
				}
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)

				assert.Equal(t, tc.expectedRequestCount, requestCount.Load(), "upstream request count")
				assert.Equal(t, tc.expectedStatusCode, w.Code, "response status code")
				assert.Equal(t, tc.expectedRetryAfter, w.Header().Get("Retry-After"), "Retry-After header")
				if pool != nil {
					assert.Equal(t, tc.expectedKeyStates, pool.PoolState(), "key states")
				}
			})
		}
	}
}
