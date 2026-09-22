package proxy

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/tracing"
	codertestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type forwardingTestProvider struct {
	*testutil.MockProvider
	failover keypool.KeyFailoverConfig
}

func (p *forwardingTestProvider) KeyFailoverConfig(slog.Logger) keypool.KeyFailoverConfig {
	return p.failover
}

func TestForwardingHandlerForwardsStreamingRequest(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 1)
	upstreamResult := make(chan error, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		body, err := io.ReadAll(r.Body)
		if err == nil && string(body) != "payload" {
			err = xerrors.Errorf("unexpected body %q", body)
		}
		upstreamResult <- err
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	prov := &forwardingTestProvider{MockProvider: &testutil.MockProvider{NameStr: "test", URL: upstream.URL}}
	handler := newTestForwardingHandler(t, prov, nil, "/")

	reader, writer := io.Pipe()
	done := make(chan int, 1)
	go func() {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/test/stream", reader))
		done <- response.Code
	}()

	ctx := codertestutil.Context(t, codertestutil.WaitShort)
	codertestutil.RequireReceive(ctx, t, started)
	_, err := writer.Write([]byte("payload"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.Equal(t, http.StatusNoContent, codertestutil.RequireReceive(ctx, t, done))
	require.NoError(t, codertestutil.RequireReceive(ctx, t, upstreamResult))
}

func TestForwardingHandlerKeyFailoverReplaysBody(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New(config.ProviderOpenAI, []string{"first-key", "second-key"}, quartz.NewMock(t), nil)
	require.NoError(t, err)
	attempts := make(chan string, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, readErr.Error(), http.StatusBadRequest)
			return
		}
		attempts <- r.Header.Get("Authorization") + ":" + string(body)
		if len(attempts) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	prov := provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL, KeyPool: pool})
	handler := newTestForwardingHandler(t, prov, nil, "/v1/chat/completions")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader("payload")))
	require.Equal(t, http.StatusNoContent, response.Code)
	require.Equal(t, "Bearer first-key:payload", <-attempts)
	require.Equal(t, "Bearer second-key:payload", <-attempts)
}

func TestForwardingHandlerRejectsUpstreamUpgrade(t *testing.T) {
	t.Parallel()

	body := &closeTrackingBody{Reader: strings.NewReader("upgrade")}
	prov := &forwardingTestProvider{MockProvider: &testutil.MockProvider{NameStr: "test", URL: "https://upstream.example.test"}}
	handler := &forwardingHandler{
		provider: prov,
		baseURL:  mustParseURL(t, prov.BaseURL()),
		transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusSwitchingProtocols,
				Header:     http.Header{"Connection": {"Upgrade"}, "Upgrade": {"h2c"}},
				Body:       body,
			}, nil
		}),
		logger:      codertestutil.NewFakeSink(t).Logger(),
		tracer:      noop.NewTracerProvider().Tracer(t.Name()),
		metricRoute: "/",
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/test/models", nil))
	require.Equal(t, http.StatusBadGateway, response.Code)
	require.True(t, body.closed)
}

func TestForwardingHandlerRequestSemantics(t *testing.T) {
	t.Parallel()

	type upstreamRequest struct {
		path     string
		rawQuery string
		header   http.Header
	}
	requests := make(chan upstreamRequest, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- upstreamRequest{path: r.URL.EscapedPath(), rawQuery: r.URL.RawQuery, header: r.Header.Clone()}
		w.Header().Set("Set-Cookie", "coder_session_token=upstream")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("response"))
	}))
	t.Cleanup(upstream.Close)
	prov := &forwardingTestProvider{MockProvider: &testutil.MockProvider{NameStr: "test", URL: upstream.URL + "/base?configured=1"}}
	handler := newTestForwardingHandler(t, prov, nil, "/")

	for _, userAgent := range []string{"", "client/1.0"} {
		req := httptest.NewRequest(http.MethodPost, "/test/models/a%2Fb?raw=a;b", strings.NewReader("body"))
		req.Header.Set("Authorization", "Bearer provider-key")
		req.Header.Set("X-Api-Key", "provider-key")
		req.Header.Set("Accept-Encoding", "gzip")
		req.Header.Set("Cookie", "coder_session_token=secret")
		req.Header.Set("Coder-Session-Token", "secret")
		req.Header.Set("X-AI-Bridge-Actor-Id", "spoofed")
		req.Header.Set("Forwarded", "for=192.0.2.1")
		req.Header.Set("X-Forwarded-For", "192.0.2.1")
		if userAgent != "" {
			req.Header.Set("User-Agent", userAgent)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		require.Equal(t, http.StatusCreated, response.Code)
		require.Equal(t, "response", response.Body.String())
		require.Empty(t, response.Header().Values("Set-Cookie"))

		got := <-requests
		require.Equal(t, "/base/models/a%2Fb", got.path)
		require.Equal(t, "configured=1&raw=a;b", got.rawQuery)
		require.Equal(t, userAgent, got.header.Get("User-Agent"))
		require.Equal(t, "gzip", got.header.Get("Accept-Encoding"))
		require.Equal(t, "Bearer provider-key", got.header.Get("Authorization"))
		require.Equal(t, "provider-key", got.header.Get("X-Api-Key"))
		for name := range got.header {
			require.NotRegexp(t, `(?i)^(cookie$|coder-|x-coder-|x-ai-bridge-actor|forwarded$|x-forwarded-)`, name)
		}
	}
}

func TestForwardingHandlerRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		prepare    func(*http.Request)
		wantStatus int
		wantBody   string
	}{
		{
			name: "Upgrade",
			prepare: func(r *http.Request) {
				r.Header.Set("Connection", "Upgrade")
				r.Header.Set("Upgrade", "h2c")
			},
			wantStatus: http.StatusNotImplemented,
			wantBody:   "HTTP upgrades are not supported",
		},
		{
			name: "Firewall",
			prepare: func(r *http.Request) {
				r.Header.Set("X-Coder-Agent-Firewall-Session-Id", "not-a-uuid")
				r.Header.Set("X-Coder-Agent-Firewall-Sequence-Number", "1")
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid agent firewall headers",
		},
		{
			name: "Traversal",
			prepare: func(r *http.Request) {
				r.URL = mustParseURL(t, "/test/models/%2e%2e/admin")
			},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid request path",
		},
		{
			name: "DeclaredOversize",
			prepare: func(r *http.Request) {
				r.ContentLength = routing.MaxRequestBodyBytes + 1
			},
			wantStatus: http.StatusRequestEntityTooLarge,
			wantBody:   "Request body too large",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			upstreamCalls := atomic.Int32{}
			upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { upstreamCalls.Add(1) }))
			t.Cleanup(upstream.Close)
			prov := &forwardingTestProvider{MockProvider: &testutil.MockProvider{NameStr: "test", URL: upstream.URL}}
			handler := newTestForwardingHandler(t, prov, nil, "/")
			req := httptest.NewRequest(http.MethodPost, "/test/models", strings.NewReader("body"))
			tc.prepare(req)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			require.Equal(t, tc.wantStatus, response.Code)
			require.Contains(t, response.Body.String(), tc.wantBody)
			require.Zero(t, upstreamCalls.Load())
		})
	}
}

func TestForwardingHandlerUnknownLengthOversizeReturns413(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.Copy(io.Discard, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	prov := &forwardingTestProvider{MockProvider: &testutil.MockProvider{NameStr: "test", URL: upstream.URL}}
	handler := newTestForwardingHandler(t, prov, nil, "/")
	body := &closeTrackingBody{Reader: io.LimitReader(zeroReader{}, routing.MaxRequestBodyBytes+1)}
	req := httptest.NewRequest(http.MethodPost, "/test/models", body)
	req.ContentLength = -1
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
	require.True(t, body.closed)
}

func TestForwardingHandlerDropsTrailers(t *testing.T) {
	t.Parallel()

	upstreamResult := make(chan error, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err == nil && len(r.Trailer) != 0 {
			err = xerrors.Errorf("unexpected request trailers: %v", r.Trailer)
		}
		upstreamResult <- err
		w.Header().Add("Trailer", "Set-Cookie")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response"))
		w.Header().Set("Set-Cookie", "upstream-trailer")
	}))
	t.Cleanup(upstream.Close)
	prov := &forwardingTestProvider{MockProvider: &testutil.MockProvider{NameStr: "test", URL: upstream.URL}}
	handler := newTestForwardingHandler(t, prov, nil, "/")

	trailers := http.Header{"Cookie": nil, "X-AI-Bridge-Actor-Id": nil}
	body := &lateTrailerReader{Reader: strings.NewReader("body"), trailer: trailers, values: http.Header{"Cookie": {"secret"}, "X-AI-Bridge-Actor-Id": {"spoofed"}}}
	req := httptest.NewRequest(http.MethodPost, "/test/models", body)
	req.ContentLength = -1
	req.Trailer = trailers
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	require.Equal(t, http.StatusOK, response.Code)
	require.Empty(t, response.Header().Values("Set-Cookie"))
	require.NoError(t, <-upstreamResult)
}

func TestForwardingHandlerWriteFailureAborts(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response"))
	}))
	t.Cleanup(upstream.Close)
	prov := &forwardingTestProvider{MockProvider: &testutil.MockProvider{NameStr: "test", URL: upstream.URL}}
	handler := newTestForwardingHandler(t, prov, nil, "/")
	writer := &failingResponseWriter{header: http.Header{}, err: xerrors.New("client disconnected")}
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		handler.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/test/models", nil))
	})
}

func TestForwardingHandlerLogsAndTracesTransportFailure(t *testing.T) {
	t.Parallel()

	sink := codertestutil.NewFakeSink(t)
	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() { require.NoError(t, tracerProvider.Shutdown(context.Background())) })
	prov := &forwardingTestProvider{MockProvider: &testutil.MockProvider{NameStr: "test", URL: "http://127.0.0.1:1"}}
	handler, transport, err := newForwardingHandler(prov, sink.Logger(), nil, tracerProvider.Tracer(t.Name()), "/")
	require.NoError(t, err)
	t.Cleanup(transport.CloseIdleConnections)
	req := httptest.NewRequest(http.MethodPost, "/test/models", strings.NewReader("body-secret"))
	req.Header.Set("Authorization", "Bearer header-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	require.Equal(t, http.StatusBadGateway, response.Code)
	require.Contains(t, fmt.Sprint(sink.Entries()), "reverse proxy error")
	require.NotContains(t, fmt.Sprint(sink.Entries()), "header-secret")
	require.NotContains(t, fmt.Sprint(sink.Entries()), "body-secret")
	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, codes.Error, spans[0].Status().Code)
	require.Contains(t, spans[0].Attributes(), attribute.String(tracing.PassthroughUpstreamURL, "http://127.0.0.1:1/models"))
}

func TestForwardingHandlerMetricLabels(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	t.Cleanup(upstream.Close)
	registry := prometheus.NewRegistry()
	m := metrics.NewMetrics(registry)
	prov := &forwardingTestProvider{MockProvider: &testutil.MockProvider{NameStr: "test", URL: upstream.URL}}
	handler := newTestForwardingHandler(t, prov, m, "/")
	for _, method := range []string{http.MethodGet, "CUSTOM"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, "/test/models", nil))
	}
	require.Equal(t, 1.0, promtest.ToFloat64(m.PassthroughCount.WithLabelValues("test", "/", http.MethodGet)))
	require.Equal(t, 1.0, promtest.ToFloat64(m.PassthroughCount.WithLabelValues("test", "/", "OTHER")))
}

func TestNewForwardingHandlerValidation(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		prov        *forwardingTestProvider
		errContains string
	}{
		{name: "RelativeBaseURL", prov: &forwardingTestProvider{MockProvider: &testutil.MockProvider{NameStr: "test", URL: "/relative"}}, errContains: "absolute HTTP or HTTPS URL"},
		{name: "MalformedBaseURL", prov: &forwardingTestProvider{MockProvider: &testutil.MockProvider{NameStr: "test", URL: "://bad"}}, errContains: "base URL"},
		{name: "MissingIsBYOK", prov: providerWithPool(t), errContains: "IsBYOK callback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			handler, transport, err := newForwardingHandler(tc.prov, codertestutil.NewFakeSink(t).Logger(), nil, nil, "/")
			require.ErrorContains(t, err, tc.errContains)
			require.Nil(t, handler)
			require.Nil(t, transport)
		})
	}
}

func newTestForwardingHandler(t *testing.T, prov provider.Provider, m *metrics.Metrics, metricRoute string) *forwardingHandler {
	t.Helper()
	logger := codertestutil.NewFakeSink(t).Logger()
	handler, transport, err := newForwardingHandler(prov, logger, m, nil, metricRoute)
	require.NoError(t, err)
	t.Cleanup(transport.CloseIdleConnections)
	return handler
}

func providerWithPool(t *testing.T) *forwardingTestProvider {
	t.Helper()
	return &forwardingTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "test", URL: "https://upstream.example.test"},
		failover:     keypool.KeyFailoverConfig{Pool: testutil.SingleKeyPool(config.ProviderOpenAI, "key")},
	}
}

type closeTrackingBody struct {
	io.Reader
	closed bool
}

func (b *closeTrackingBody) Close() error {
	b.closed = true
	return nil
}

type lateTrailerReader struct {
	io.Reader
	trailer http.Header
	values  http.Header
	set     bool
}

func (r *lateTrailerReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF && !r.set {
		maps.Copy(r.trailer, r.values)
		r.set = true
	}
	return n, err
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}
