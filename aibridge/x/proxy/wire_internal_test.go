package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept/apidump"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/utils"
	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// newTestForwardingHandler constructs a forwarding handler over a transport
// configured like the router's, and releases the transport after the test.
func newTestForwardingHandler(t *testing.T, prov provider.Provider, m *metrics.Metrics) *forwardingHandler {
	t.Helper()
	logger := slogtest.Make(t, nil)
	gate := aibridge.NewInflightGate(logger)
	t.Cleanup(func() {
		defer gate.Close()
		require.NoError(t, gate.Shutdown(codertestutil.Context(t, codertestutil.WaitLong)))
	})
	transport := utils.NewStreamingTransport()
	transport.DisableCompression = true
	t.Cleanup(transport.CloseIdleConnections)
	h, err := newForwardingHandler(prov, logger, m, noop.NewTracerProvider().Tracer(t.Name()), gate, &struct{ recorder.Recorder }{}, transport)
	require.NoError(t, err)
	require.NotNil(t, h.proxy)
	return h
}

// prepareAndProxy validates r, prepares it with the resolved credential, and
// forwards it with response observation outside the circuit breaker. It
// returns the prepared request and its replay buffer.
func prepareAndProxy(t *testing.T, h *forwardingHandler, w http.ResponseWriter, r *http.Request) (*http.Request, *requestBuffer) {
	t.Helper()
	record, cred := h.checkRequest(w, r)
	require.NotNil(t, record, "request must pass validation")
	state := &responseObservation{credentialHint: record.CredentialHint, client: w}
	outbound, body := h.prepareForwarding(r.WithContext(context.WithValue(r.Context(), observationContextKey{}, state)), cred)
	_ = h.forwardPrepared(w, outbound, body, state)
	return outbound, body
}

func TestNewForwardingHandlerMalformedBaseURL(t *testing.T) {
	t.Parallel()
	prov := provider.NewOpenAI(config.OpenAI{BaseURL: "http://upstream.example.test/%zz"})
	logger := slogtest.Make(t, nil)
	gate := aibridge.NewInflightGate(logger)
	t.Cleanup(func() {
		defer gate.Close()
		require.NoError(t, gate.Shutdown(codertestutil.Context(t, codertestutil.WaitLong)))
	})
	tracer := noop.NewTracerProvider().Tracer(t.Name())
	rec := &struct{ recorder.Recorder }{}

	h, err := newForwardingHandler(prov, logger, nil, tracer, gate, rec, http.DefaultTransport)
	require.ErrorContains(t, err, `configure provider "openai" base URL`)
	require.Nil(t, h)

	router, err := NewRouter(t.Context(), []provider.Provider{prov}, logger, nil, tracer, gate, rec)
	require.ErrorContains(t, err, `configure provider "openai" base URL`)
	require.Nil(t, router)
}

func TestForwardingWireBYOK(t *testing.T) {
	t.Parallel()
	const payload = "  not JSON\x00\n"
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	_, err := gz.Write([]byte("raw response"))
	require.NoError(t, err)
	require.NoError(t, gz.Close())

	for _, tc := range []struct {
		name          string
		userAgent     string
		encoding      string
		dump          bool
		wantUserAgent string
	}{
		// Without a client encoding, the transport must not request or decode
		// compression, so the upstream bytes reach the client unchanged.
		{name: "APIDump", dump: true},
		{name: "ClientHeaders", userAgent: "claude-code/1.0", encoding: "gzip", wantUserAgent: "claude-code/1.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			auth := make(chan string, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case auth <- r.Header.Get("Authorization"):
				default:
					t.Error("BYOK forwarding must not retry")
				}
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, payload, string(body))
				assert.EqualValues(t, -1, r.ContentLength)
				assert.Equal(t, "/base/models/a%2Fb", r.URL.EscapedPath())
				assert.Equal(t, "configured=1&raw=a;b", r.URL.RawQuery)
				assert.Equal(t, tc.wantUserAgent, r.UserAgent())
				assert.Equal(t, tc.encoding, r.Header.Get("Accept-Encoding"))
				w.Header().Set("Content-Encoding", "gzip")
				_, _ = w.Write(compressed.Bytes())
			}))
			t.Cleanup(upstream.Close)
			dumpDir := ""
			if tc.dump {
				dumpDir = t.TempDir()
			}
			h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL + "/base?configured=1", APIDumpDir: dumpDir}), metrics.NewMetrics(prometheus.NewRegistry()))
			input := &trackedBody{Reader: strings.NewReader(payload)}
			req := requestWithAuth(t, input)
			req.URL.Path = "/openai/v1/models/a/b"
			req.URL.RawPath = "/openai/v1/models/a%2Fb"
			req.URL.RawQuery = "raw=a;b"
			if tc.userAgent != "" {
				req.Header.Set("User-Agent", tc.userAgent)
			}
			if tc.encoding != "" {
				req.Header.Set("Accept-Encoding", tc.encoding)
			}
			original := req.Header.Clone()

			response := httptest.NewRecorder()
			_, body := prepareAndProxy(t, h, response, req)
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, compressed.Bytes(), response.Body.Bytes())
			require.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
			require.Equal(t, "Bearer user-secret-key", codertestutil.RequireReceive(codertestutil.Context(t, codertestutil.WaitShort), t, auth))
			require.Equal(t, original, req.Header, "preparation must not mutate the inbound headers")
			require.Equal(t, 1, input.eofs)
			require.Zero(t, input.closes, "the server owns the inbound body")

			replay, err := body.getBody()
			require.NoError(t, err)
			replayed, err := io.ReadAll(replay)
			require.NoError(t, err)
			require.Equal(t, payload, string(replayed))
			require.Equal(t, 1, input.eofs, "replay must not read completed input again")

			if !tc.dump {
				return
			}
			for _, suffix := range []string{apidump.SuffixRequest, apidump.SuffixResponse} {
				files, err := filepath.Glob(filepath.Join(dumpDir, "openai", "passthrough", "*"+suffix))
				require.NoError(t, err)
				require.Len(t, files, 1)
				data, err := os.ReadFile(files[0])
				require.NoError(t, err)
				require.NotContains(t, string(data), "user-secret-key")
				if suffix == apidump.SuffixRequest {
					require.Contains(t, string(data), "Authorization: "+utils.MaskSecret("Bearer user-secret-key"))
					require.Contains(t, string(data), payload)
				} else {
					require.Contains(t, string(data), "HTTP/1.1 200 OK")
				}
			}
		})
	}
}

// Client-selected credentials replace competing auth headers and take
// precedence over configured key pools.
func TestForwardingWireProviderCredentials(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"OpenAIConnectionAuth", "AnthropicAPIKey", "CopilotBearer"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			received := make(chan http.Header, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := io.Copy(io.Discard, r.Body)
				assert.NoError(t, err)
				select {
				case received <- r.Header.Clone():
				default:
					t.Error("BYOK forwarding must not retry")
				}
				w.WriteHeader(http.StatusUnauthorized)
			}))
			t.Cleanup(upstream.Close)
			pool, err := keypool.New("test", []string{"pool-secret-key"}, quartz.NewMock(t), nil)
			require.NoError(t, err)
			var prov provider.Provider = provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL, KeyPool: pool})
			req := requestWithAuth(t, strings.NewReader("body"))
			wantAuth := http.Header{"Authorization": {"Bearer user-secret-key"}}
			switch name {
			case "OpenAIConnectionAuth":
				// ReverseProxy removes headers nominated by Connection.
				req.Header.Set("Connection", "Authorization")
			case "AnthropicAPIKey":
				prov, err = provider.NewAnthropic(t.Context(), config.Anthropic{BaseURL: upstream.URL, KeyPool: pool}, nil, nil)
				require.NoError(t, err)
				req.Header.Set("X-Api-Key", "anthropic-secret-key")
				wantAuth = http.Header{"X-Api-Key": {"anthropic-secret-key"}}
			case "CopilotBearer":
				prov = provider.NewCopilot(config.Copilot{BaseURL: upstream.URL})
			}
			req.URL.Path = prov.RoutePrefix() + "/messages"
			h := newTestForwardingHandler(t, prov, nil)

			response := httptest.NewRecorder()
			prepareAndProxy(t, h, response, req)
			require.Equal(t, http.StatusUnauthorized, response.Code)
			got := codertestutil.RequireReceive(codertestutil.Context(t, codertestutil.WaitShort), t, received)
			for _, header := range []string{"Authorization", "X-Api-Key"} {
				require.Equal(t, wantAuth.Values(header), got.Values(header), header)
			}
		})
	}
}

// Requests without a body stay bodiless upstream, and a nonnil body with a
// zero length is forwarded as unknown length instead of being dropped.
func TestForwardingWireRequestBodyLength(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		body          io.ReadCloser
		contentLength int64
		wantPrepared  int64
		wantUpstream  int64
		wantPayload   string
	}{
		{name: "Nil", wantUpstream: 0},
		{name: "NoBody", body: http.NoBody, wantUpstream: 0},
		{name: "UnknownLengthZero", body: io.NopCloser(strings.NewReader("payload")), wantPrepared: -1, wantUpstream: -1, wantPayload: "payload"},
		{name: "UnknownLength", body: io.NopCloser(strings.NewReader("payload")), contentLength: -1, wantPrepared: -1, wantUpstream: -1, wantPayload: "payload"},
		{name: "KnownLength", body: io.NopCloser(strings.NewReader("payload")), contentLength: 7, wantPrepared: 7, wantUpstream: 7, wantPayload: "payload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				payload, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, tc.wantPayload, string(payload))
				assert.Equal(t, tc.wantUpstream, r.ContentLength)
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(upstream.Close)
			h := newTestForwardingHandler(t, provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL}), nil)
			req := requestWithAuth(t, nil)
			req.Body = tc.body
			req.ContentLength = tc.contentLength

			response := httptest.NewRecorder()
			outbound, body := prepareAndProxy(t, h, response, req)
			require.Equal(t, http.StatusNoContent, response.Code)
			require.Equal(t, tc.wantPrepared, outbound.ContentLength)
			require.Equal(t, tc.contentLength, req.ContentLength, "preparation must not mutate the inbound request")
			require.NotNil(t, outbound.GetBody)
			replay, err := body.getBody()
			require.NoError(t, err)
			if tc.wantPayload == "" {
				require.Equal(t, http.NoBody, outbound.Body)
				require.Equal(t, http.NoBody, replay, "known empty bodies must not become chunked requests")
				return
			}
			replayed, err := io.ReadAll(replay)
			require.NoError(t, err)
			require.Equal(t, tc.wantPayload, string(replayed))
		})
	}
}

func TestRouterBridgedTransportReuse(t *testing.T) {
	t.Parallel()
	remotes := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.Copy(io.Discard, r.Body)
		assert.NoError(t, err)
		assert.Empty(t, r.Header.Get("Accept-Encoding"), "the router transport must not request compression")
		remotes <- r.RemoteAddr
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(upstream.Close)
	logger := slogtest.Make(t, nil)
	gate := aibridge.NewInflightGate(logger)
	t.Cleanup(func() {
		defer gate.Close()
		require.NoError(t, gate.Shutdown(codertestutil.Context(t, codertestutil.WaitLong)))
	})
	router, err := NewRouter(t.Context(), []provider.Provider{provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL})}, logger, nil, noop.NewTracerProvider().Tracer(t.Name()), gate, &struct{ recorder.Recorder }{})
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	var shared *forwardingHandler
	// The public bridged handler is dormant, so drive the routed handler's
	// forwarding seam directly.
	send := func(path string) string {
		t.Helper()
		req := requestWithAuth(t, strings.NewReader("body"))
		req.URL.Path = path
		handler, _ := router.mux.Handler(req)
		h, ok := handler.(*forwardingHandler)
		require.True(t, ok, "%s must route to bridged forwarding", path)
		if shared == nil {
			shared = h
		}
		require.Same(t, shared, h, "bridged routes must share the provider's forwarding handler")
		response := httptest.NewRecorder()
		prepareAndProxy(t, h, response, req)
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, "ok", response.Body.String())
		return codertestutil.RequireReceive(ctx, t, remotes)
	}
	first := send("/openai/v1/chat/completions")
	require.Equal(t, first, send("/openai/v1/responses"), "bridged routes must reuse the provider connection")
	router.CloseIdleConnections()
	require.NotEqual(t, first, send("/openai/v1/chat/completions"), "closing idle connections must force a new connection")
}
