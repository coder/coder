package aibridge

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
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

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

var testTracer = otel.Tracer("bridge_test")

type lateTrailerReader struct {
	io.Reader
	trailer http.Header
	values  http.Header
	set     bool
}

func (r *lateTrailerReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF && !r.set {
		for name, values := range r.values {
			r.trailer[name] = append([]string(nil), values...)
		}
		r.set = true
	}
	return n, err
}

func TestPassthroughSupportsProtocolUpgrade(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !assert.True(t, ok) {
			return
		}
		conn, rw, err := hijacker.Hijack()
		if !assert.NoError(t, err) {
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: test\r\n\r\n")
		_ = rw.Flush()
		buf := make([]byte, 4)
		_, err = io.ReadFull(rw, buf)
		if !assert.NoError(t, err) {
			return
		}
		_, _ = rw.Write(buf)
		_ = rw.Flush()
	}))
	t.Cleanup(upstream.Close)

	prov := &testutil.MockProvider{NameStr: "test", URL: upstream.URL}
	handler := newPassthroughRouter(prov, slogtest.Make(t, nil), nil, testTracer)
	gateway := httptest.NewServer(handler)
	t.Cleanup(gateway.Close)

	gatewayURL, err := url.Parse(gateway.URL)
	require.NoError(t, err)
	conn, err := net.Dial("tcp", gatewayURL.Host)
	require.NoError(t, err)
	defer conn.Close()
	_, err = fmt.Fprintf(conn, "GET /upgrade HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: test\r\n\r\n", gatewayURL.Host)
	require.NoError(t, err)
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	resp, err := http.ReadResponse(rw.Reader, &http.Request{Method: http.MethodGet})
	require.NoError(t, err)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, err = conn.Write([]byte("ping"))
	require.NoError(t, err)
	echo := make([]byte, 4)
	_, err = io.ReadFull(conn, echo)
	require.NoError(t, err)
	require.Equal(t, "ping", string(echo))
}

func TestPassthroughDropsRequestAndResponseTrailers(t *testing.T) {
	t.Parallel()

	const body = "request-body"
	upstreamResult := make(chan error, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, err := io.ReadAll(r.Body)
		if err == nil && string(gotBody) != body {
			err = xerrors.Errorf("unexpected body %q", gotBody)
		}
		if err == nil && len(r.Trailer) != 0 {
			err = xerrors.Errorf("unexpected request trailers: %v", r.Trailer)
		}
		upstreamResult <- err
		w.Header().Add("Trailer", "Set-Cookie")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response-body"))
		w.Header().Set("Set-Cookie", "coder_session_token=upstream-trailer")
	}))
	t.Cleanup(upstream.Close)

	prov := &testutil.MockProvider{NameStr: "test", URL: upstream.URL}
	handler := newPassthroughRouter(prov, slogtest.Make(t, nil), nil, testTracer)
	gateway := httptest.NewServer(handler)
	t.Cleanup(gateway.Close)
	requestTrailers := http.Header{
		"Cookie":               nil,
		"X-AI-Bridge-Actor-Id": nil,
	}
	requestBody := &lateTrailerReader{
		Reader:  strings.NewReader(body),
		trailer: requestTrailers,
		values: http.Header{
			"Cookie":               {"coder_session_token=request-trailer"},
			"X-AI-Bridge-Actor-Id": {"spoofed"},
		},
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, gateway.URL+"/v1/models", requestBody)
	require.NoError(t, err)
	req.ContentLength = -1
	req.Trailer = requestTrailers
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	gotBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "response-body", string(gotBody))
	require.Empty(t, resp.Trailer)
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	require.NoError(t, codertestutil.RequireReceive(ctx, t, upstreamResult))
}

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
			name:          "sanitizes_sensitive_and_untrusted_proxy_headers",
			reqPath:       "/v1/models",
			reqHost:       "client.example.com",
			reqRemoteAddr: "1.1.1.1:1111",
			reqHeaders: http.Header{
				"Cookie":                      {"coder_session_token=secret"},
				"Coder-Session-Token":         {"secret"},
				"X-Coder-AI-Governance-Token": {"secret"},
				"X-AI-Bridge-Actor-Id":        {"spoofed"},
				"Forwarded":                   {"for=2.2.2.2"},
				"X-Forwarded-For":             {"2.2.2.2"},
				"X-Forwarded-Server":          {"client"},
			},
			expectRequestPath: "/v1/models",
			expectRespStatus:  http.StatusOK,
			expectRespBody:    upstreamRespBody,
			expectHeaders: http.Header{
				"Accept-Encoding":   {"gzip"},
				"User-Agent":        {"aibridge"},
				"X-Forwarded-For":   {"1.1.1.1"},
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
			name:          "bare_IPv4_remote_addr_sets_forwarded_for",
			reqPath:       "http://client-host/chat",
			reqRemoteAddr: "1.1.1.1",
			reqHeaders: http.Header{
				"X-Forwarded-For": {"203.0.113.10"},
			},
			provider:  &testutil.MockProvider{URL: "https://upstream-host/base"},
			expectURL: "https://upstream-host/base/chat",
			expectHeaders: http.Header{
				"X-Forwarded-Host":  {"client-host"},
				"X-Forwarded-Proto": {"http"},
				"X-Forwarded-For":   {"1.1.1.1"},
				"User-Agent":        {"aibridge"},
			},
		},
		{
			name:          "bare_IPv6_remote_addr_sets_forwarded_for",
			reqPath:       "http://client-host/chat",
			reqRemoteAddr: "2001:db8::1",
			provider:      &testutil.MockProvider{URL: "https://upstream-host/base"},
			expectURL:     "https://upstream-host/base/chat",
			expectHeaders: http.Header{
				"X-Forwarded-Host":  {"client-host"},
				"X-Forwarded-Proto": {"http"},
				"X-Forwarded-For":   {"2001:db8::1"},
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
			// Incoming forwarding chains are untrusted. The proxy starts a new chain
			// from its directly connected client peer.
			name:          "replaces_existing_forwarded_for_chain",
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
				"X-Forwarded-For":   {"1.1.1.1"},
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

			originalIn := pr.In
			originalRemoteAddr := pr.In.RemoteAddr
			rewritePassthroughRequest(pr, provBaseURL)

			assert.Same(t, originalIn, pr.In)
			assert.Equal(t, originalRemoteAddr, pr.In.RemoteAddr)
			assert.Equal(t, tc.expectURL, pr.Out.URL.String())
			assert.Equal(t, "", pr.Out.Host)
			assert.Equal(t, tc.expectHeaders, pr.Out.Header)
		})
	}
}

func TestPassthroughRejectsTraversalAndPreservesEscapedSeparator(t *testing.T) {
	t.Parallel()

	upstreamPaths := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPaths <- r.URL.EscapedPath()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	prov := &testutil.MockProvider{NameStr: "test", URL: upstream.URL}
	handler := newPassthroughRouter(prov, slogtest.Make(t, nil), nil, testTracer)

	for _, path := range []string{"/v1/models/%2e%2e/admin", "/v1/models/%2E/admin"} {
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusBadRequest, resp.Code)
	}

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/models/a%2Fb", nil))
	require.Equal(t, http.StatusNoContent, resp.Code)
	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	require.Equal(t, "/v1/models/a%2Fb", codertestutil.RequireReceive(ctx, t, upstreamPaths))
}

func TestPassthroughStripsSetCookie(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Set-Cookie", "coder_session_token=upstream")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	prov := &testutil.MockProvider{NameStr: "test", URL: upstream.URL}
	handler := newPassthroughRouter(prov, slogtest.Make(t, nil), nil, testTracer)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	require.Empty(t, resp.Header().Values("Set-Cookie"))
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

func TestPassthroughMetricCardinalityIsBounded(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	registry := prometheus.NewRegistry()
	m := NewMetrics(registry)
	prov := provider.NewCopilot(config.Copilot{BaseURL: upstream.URL})
	handler := newPassthroughRouter(prov, slogtest.Make(t, nil), m, testTracer)

	for i := range 50 {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/v1/threads/%d/messages", i), nil)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		require.Equal(t, http.StatusNoContent, resp.Code)
	}

	require.Equal(t, 50.0, promtest.ToFloat64(m.PassthroughCount.WithLabelValues(config.ProviderCopilot, "/", http.MethodGet)))
	require.Equal(t, 1, promtest.CollectAndCount(m.PassthroughCount))
}

// TestPassthrough_KeyFailover exercises the KeyFailoverTransport
// end-to-end through the passthrough proxy, parameterised over
// providers (anthropic, openai, copilot). Each scenario asserts the
// response status and Retry-After, the keys the upstream actually
// saw, and the final pool state.
func TestPassthrough_KeyFailover(t *testing.T) {
	t.Parallel()

	// providers parameterises the table over the providers exposed
	// to the failover transport. Each entry encapsulates the
	// provider-specific bits the test needs: how a BYOK request
	// sets its auth header and how the provider is constructed for
	// a given pool.
	providers := []struct {
		name        string
		byokOnly    bool
		setBYOK     func(*http.Request, string)
		newProvider func(baseURL string, pool *keypool.Pool) provider.Provider
	}{
		{
			name: "anthropic",
			setBYOK: func(r *http.Request, key string) {
				r.Header.Set("X-Api-Key", key)
			},
			newProvider: func(baseURL string, pool *keypool.Pool) provider.Provider {
				p, err := provider.NewAnthropic(context.Background(), config.Anthropic{
					BaseURL: baseURL,
					KeyPool: pool,
				}, nil)
				require.NoError(t, err)
				return p
			},
		},
		{
			name: "openai",
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
		// Copilot is BYOK-only: its KeyFailoverConfig is zero-value
		// so the failover transport short-circuits.
		{
			name:     "copilot",
			byokOnly: true,
			setBYOK: func(r *http.Request, key string) {
				r.Header.Set("Authorization", "Bearer "+key)
			},
			newProvider: func(baseURL string, _ *keypool.Pool) provider.Provider {
				return provider.NewCopilot(config.Copilot{BaseURL: baseURL})
			},
		},
	}

	tests := []struct {
		name string
		// Centralized pool keys. Empty when byokKey is set.
		keys []string
		// BYOK key. Empty when keys is set.
		byokKey string
		// Sequential upstream responses replayed by MockUpstream
		// in call order. MockUpstream's strict mode asserts the
		// upstream call count matches len(upstreamResponses).
		upstreamResponses []testutil.UpstreamResponse
		// Expected keys the upstream actually saw, in call order.
		expectedSeenKeys   []string
		expectedStatusCode int
		expectedRetryAfter string
		// Expected key states after the request, by index in keys.
		expectedKeyStates []keypool.KeyState
		// Expected key_pool_state_transitions_total counts by reason.
		expectedTransitions map[string]int
		// Expected key_pool_exhaustions_total counts by outcome.
		expectedExhaustions map[string]int
	}{
		{
			// Given: 1 valid key returning 200.
			// Then: 1 request, 200 response, key remains valid.
			name: "single_valid_key",
			keys: []string{"k0"},
			upstreamResponses: []testutil.UpstreamResponse{
				{Blocking: []byte("{}")},
			},
			expectedSeenKeys:   []string{"k0"},
			expectedStatusCode: http.StatusOK,
			expectedKeyStates:  []keypool.KeyState{keypool.KeyStateValid},
		},
		{
			// Given: 2 keys; key-0 returns 429, key-1 returns 200.
			// Then: 2 requests, 200 response, key-0 temporary, key-1 valid.
			name: "failover_after_429",
			keys: []string{"k0", "k1"},
			upstreamResponses: []testutil.UpstreamResponse{
				testutil.NewErrorResponse(http.StatusTooManyRequests, "5"),
				{Blocking: []byte("{}")},
			},
			expectedSeenKeys:   []string{"k0", "k1"},
			expectedStatusCode: http.StatusOK,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateValid,
			},
			expectedTransitions: map[string]int{"rate_limited": 1},
		},
		{
			// Given: 2 keys; key-0 returns 401, key-1 returns 200.
			// Then: 2 requests, 200 response, key-0 temporary, key-1 valid.
			name: "failover_after_401",
			keys: []string{"k0", "k1"},
			upstreamResponses: []testutil.UpstreamResponse{
				testutil.NewErrorResponse(http.StatusUnauthorized, ""),
				{Blocking: []byte("{}")},
			},
			expectedSeenKeys:   []string{"k0", "k1"},
			expectedStatusCode: http.StatusOK,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateValid,
			},
			expectedTransitions: map[string]int{"unauthorized": 1},
		},
		{
			// Given: 3 keys; all return 429 with cooldowns 5s, 3s, 10s.
			// Then: 3 requests, 429 response with smallest Retry-After,
			// all keys temporary.
			name: "all_keys_temporary_blocked",
			keys: []string{"k0", "k1", "k2"},
			upstreamResponses: []testutil.UpstreamResponse{
				testutil.NewErrorResponse(http.StatusTooManyRequests, "5"),
				testutil.NewErrorResponse(http.StatusTooManyRequests, "3"),
				testutil.NewErrorResponse(http.StatusTooManyRequests, "10"),
			},
			expectedSeenKeys:   []string{"k0", "k1", "k2"},
			expectedStatusCode: http.StatusTooManyRequests,
			expectedRetryAfter: "3",
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateTemporary,
				keypool.KeyStateTemporary,
			},
			expectedTransitions: map[string]int{"rate_limited": 3},
			expectedExhaustions: map[string]int{"rate_limited": 1},
		},
		{
			// Given: 2 keys; both return 401.
			// Then: 2 requests, 502 auth-failure response with no Retry-After,
			// both keys temporary and recovering after the cooldown.
			name: "all_keys_unauthorized",
			keys: []string{"k0", "k1"},
			upstreamResponses: []testutil.UpstreamResponse{
				testutil.NewErrorResponse(http.StatusUnauthorized, ""),
				testutil.NewErrorResponse(http.StatusUnauthorized, ""),
			},
			expectedSeenKeys:   []string{"k0", "k1"},
			expectedStatusCode: http.StatusBadGateway,
			expectedRetryAfter: "",
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateTemporary,
				keypool.KeyStateTemporary,
			},
			expectedTransitions: map[string]int{"unauthorized": 2},
			expectedExhaustions: map[string]int{"auth_failed": 1},
		},
		{
			// Given: 2 keys; key-0 returns 403.
			// Then: 1 request, 403 surfaced as-is, both keys valid.
			name: "forbidden_no_failover",
			keys: []string{"k0", "k1"},
			upstreamResponses: []testutil.UpstreamResponse{
				testutil.NewErrorResponse(http.StatusForbidden, ""),
			},
			expectedSeenKeys:   []string{"k0"},
			expectedStatusCode: http.StatusForbidden,
			expectedKeyStates: []keypool.KeyState{
				keypool.KeyStateValid,
				keypool.KeyStateValid,
			},
		},
		{
			// Given: 2 keys; key-0 returns 500.
			// Then: 1 request, 500 response, both keys remain valid.
			name: "server_error_no_failover",
			keys: []string{"k0", "k1"},
			upstreamResponses: []testutil.UpstreamResponse{
				testutil.NewErrorResponse(http.StatusInternalServerError, ""),
			},
			expectedSeenKeys:   []string{"k0"},
			expectedStatusCode: http.StatusInternalServerError,
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
			upstreamResponses: []testutil.UpstreamResponse{
				testutil.NewErrorResponse(http.StatusTooManyRequests, "5"),
			},
			expectedSeenKeys:   []string{"user-byok"},
			expectedStatusCode: http.StatusTooManyRequests,
			expectedRetryAfter: "5",
		},
	}

	for _, prov := range providers {
		for _, tc := range tests {
			// BYOK-only providers don't use the pool, so pool-based
			// cases don't apply.
			if prov.byokOnly && tc.byokKey == "" {
				continue
			}
			t.Run(prov.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()

				// MockUpstream replays the scripted responses in
				// call order. Strict mode fails the test if the
				// upstream sees a different number of requests
				// than tc.upstreamResponses describes.
				upstream := testutil.NewMockUpstream(t.Context(), t, tc.upstreamResponses...)

				reg := prometheus.NewRegistry()
				m := NewMetrics(reg)

				var pool *keypool.Pool
				if len(tc.keys) > 0 {
					var err error
					pool, err = keypool.New("test", tc.keys, quartz.NewMock(t), m)
					require.NoError(t, err)
				}

				p := prov.newProvider(upstream.URL, pool)
				logger := slogtest.Make(t, nil)
				handler := newPassthroughRouter(p, logger, nil, testTracer)

				req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
				if tc.byokKey != "" {
					prov.setBYOK(req, tc.byokKey)
				}
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)

				assert.Equal(t, tc.expectedStatusCode, w.Code, "response status code")
				assert.Equal(t, tc.expectedRetryAfter, w.Header().Get("Retry-After"), "Retry-After header")

				var seenKeys []string
				for _, r := range upstream.ReceivedRequests() {
					seenKeys = append(seenKeys, testutil.KeyFromHeader(p.AuthHeader(), r.Header))
				}
				assert.Equal(t, tc.expectedSeenKeys, seenKeys, "seen keys")

				if pool != nil {
					assert.Equal(t, tc.expectedKeyStates, pool.PoolState(), "key states")

					gathered, err := reg.Gather()
					require.NoError(t, err)
					// One transition per marked key, by reason.
					for _, reason := range []string{"rate_limited", "unauthorized"} {
						if want := tc.expectedTransitions[reason]; want > 0 {
							assert.True(t, codertestutil.PromCounterHasValue(t, gathered, float64(want), "key_pool_state_transitions_total", "test", reason))
						} else {
							assert.False(t, codertestutil.PromCounterGathered(t, gathered, "key_pool_state_transitions_total", "test", reason))
						}
					}
					// Exhaustion outcome when no usable key remains.
					for _, outcome := range []string{"rate_limited", "auth_failed"} {
						if want := tc.expectedExhaustions[outcome]; want > 0 {
							assert.True(t, codertestutil.PromCounterHasValue(t, gathered, float64(want), "key_pool_exhaustions_total", outcome, "test"))
						} else {
							assert.False(t, codertestutil.PromCounterGathered(t, gathered, "key_pool_exhaustions_total", outcome, "test"))
						}
					}
					// One observation per request, summing the keys tried.
					hist := promhelp.HistogramValue(t, reg, "key_pool_failover_attempts", prometheus.Labels{"provider": "test"})
					require.NotNil(t, hist)
					assert.Equal(t, uint64(1), hist.GetSampleCount())
					assert.Equal(t, float64(len(tc.upstreamResponses)), hist.GetSampleSum())
				}
			})
		}
	}
}
