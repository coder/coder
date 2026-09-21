package proxy_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/tracing"
	"github.com/coder/coder/v2/aibridge/x/proxy"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/coderdtest/promhelp"
	codertestutil "github.com/coder/coder/v2/testutil"
)

func TestHandlerPreflightRejections(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		prepare    func(*http.Request) *http.Request
		wantStatus int
	}{
		{
			name:       "MissingActor",
			prepare:    func(r *http.Request) *http.Request { return r },
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "WebSocket",
			prepare: func(r *http.Request) *http.Request {
				r = r.WithContext(aibridge.AsActor(r.Context(), "actor", nil))
				r.Method = http.MethodGet
				r.Header.Set("Connection", "Upgrade")
				r.Header.Set("Upgrade", "websocket")
				return r
			},
			wantStatus: http.StatusNotImplemented,
		},
		{
			name: "MalformedAgentFirewallHeaders",
			prepare: func(r *http.Request) *http.Request {
				r = r.WithContext(aibridge.AsActor(r.Context(), "actor", nil))
				r.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, "not-a-uuid")
				r.Header.Set(agplaibridge.HeaderAgentFirewallSequenceNumber, "1")
				return r
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "PartialAgentFirewallHeaders",
			prepare: func(r *http.Request) *http.Request {
				r = r.WithContext(aibridge.AsActor(r.Context(), "actor", nil))
				r.Header.Set(agplaibridge.HeaderAgentFirewallSessionID, "e5f6a7b8-1234-5678-9abc-def012345678")
				return r
			},
			wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			upstreamCalls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				upstreamCalls++
			}))
			t.Cleanup(upstream.Close)
			rec := &testutil.MockRecorder{}
			provider := &proxyTestProvider{
				MockProvider: &testutil.MockProvider{NameStr: "openai", URL: upstream.URL, Bridged: []string{"/v1/chat"}},
				typ:          config.ProviderOpenAI, authHeader: "Authorization",
			}
			sink := codertestutil.NewFakeSink(t)
			router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, sink.Logger(), nil, testTracer(t))
			require.NoError(t, err)
			t.Cleanup(router.CloseIdleConnections)

			req := tc.prepare(httptest.NewRequest(http.MethodPost, "/openai/v1/chat", nil))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)

			require.Equal(t, tc.wantStatus, response.Code)
			require.Zero(t, upstreamCalls)
			require.Empty(t, rec.RecordedInterceptions())
			require.NotEmpty(t, sink.Entries())
		})
	}
}

func TestHandlerLogsRecorderStartFailureSafely(t *testing.T) {
	t.Parallel()

	const (
		secretHeader = "provider-secret-value"
		secretBody   = "body-secret-value"
	)
	sink := codertestutil.NewFakeSink(t)
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "openai", URL: "http://127.0.0.1:1", Bridged: []string{"/v1/chat"}},
		typ:          config.ProviderOpenAI, authHeader: "Authorization",
	}
	rec := &failingStartRecorder{err: xerrors.New("recording unavailable")}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, sink.Logger(), nil, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", strings.NewReader(secretBody))
	req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
	req.Header.Set("Authorization", "Bearer "+secretHeader)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	require.Equal(t, http.StatusInternalServerError, response.Code)

	entries := fmt.Sprint(sink.Entries())
	require.Contains(t, entries, "failed to record interception")
	require.NotContains(t, entries, secretHeader)
	require.NotContains(t, entries, secretBody)
}

func TestHandlerLogsFailuresSafelyAndTracesUpstream(t *testing.T) {
	t.Parallel()

	const (
		secretHeader = "provider-secret-value"
		secretBody   = "body-secret-value"
	)
	sink := codertestutil.NewFakeSink(t)
	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	t.Cleanup(func() { require.NoError(t, tracerProvider.Shutdown(context.Background())) })

	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{NameStr: "copilot", URL: "http://127.0.0.1:1", Passthrough: []string{"/"}},
		typ:          config.ProviderCopilot, authHeader: "Authorization",
	}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, nil, sink.Logger(), nil, tracerProvider.Tracer(t.Name()))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	req := httptest.NewRequest(http.MethodPost, "/copilot/other", strings.NewReader(secretBody))
	req.Header.Set("Authorization", "Bearer "+secretHeader)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	require.Equal(t, http.StatusBadGateway, response.Code)

	entries := sink.Entries()
	require.Contains(t, fmt.Sprint(entries), "reverse proxy error")
	require.NotContains(t, fmt.Sprint(entries), secretHeader)
	require.NotContains(t, fmt.Sprint(entries), secretBody)

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, codes.Error, spans[0].Status().Code)
	require.Contains(t, spans[0].Attributes(), attribute.String(tracing.PassthroughUpstreamURL, "http://127.0.0.1:1/other"))
}

func TestHandlerMetrics(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test-Failure") != "" {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_, _ = w.Write([]byte("response"))
	}))
	t.Cleanup(upstream.Close)

	registry := prometheus.NewRegistry()
	m := metrics.NewMetrics(registry)
	rec := &testutil.MockRecorder{}
	provider := &proxyTestProvider{
		MockProvider: &testutil.MockProvider{
			NameStr: "openai", URL: upstream.URL,
			Bridged: []string{"/v1/chat"}, Passthrough: []string{"/v1/models"},
		},
		typ: config.ProviderOpenAI, authHeader: "Authorization",
	}
	router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, codertestutil.NewFakeSink(t).Logger(), m, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	serve := func(header http.Header, writer http.ResponseWriter) {
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", nil)
		req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
		req.Header = header
		router.ServeHTTP(writer, req)
	}
	serve(http.Header{}, httptest.NewRecorder())
	serve(http.Header{"X-Test-Failure": {"true"}}, httptest.NewRecorder())
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		serve(http.Header{}, &failingResponseWriter{header: http.Header{}, err: io.ErrClosedPipe})
	})

	passthrough := httptest.NewRecorder()
	router.ServeHTTP(passthrough, httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil))
	require.Equal(t, http.StatusOK, passthrough.Code)
	upstream.Close()
	passthroughFailure := httptest.NewRecorder()
	router.ServeHTTP(passthroughFailure, httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil))
	require.Equal(t, http.StatusBadGateway, passthroughFailure.Code)

	require.Zero(t, promtest.ToFloat64(m.InterceptionsInflight.WithLabelValues("openai", "", "/v1/chat")))
	require.Equal(t, 1.0, promtest.ToFloat64(m.InterceptionCount.WithLabelValues("openai", "", metrics.InterceptionCountStatusCompleted, "/v1/chat", http.MethodPost, "actor", string(aibridge.ClientUnknown))))
	require.Equal(t, 2.0, promtest.ToFloat64(m.InterceptionCount.WithLabelValues("openai", "", metrics.InterceptionCountStatusFailed, "/v1/chat", http.MethodPost, "actor", string(aibridge.ClientUnknown))))
	require.Equal(t, 2.0, promtest.ToFloat64(m.PassthroughCount.WithLabelValues("openai", "/v1/models", http.MethodGet)))

	histogram := promhelp.HistogramValue(t, registry, "interceptions_duration_seconds", prometheus.Labels{
		"provider": "openai",
		"model":    "",
	})
	require.NotNil(t, histogram)
	require.EqualValues(t, 3, histogram.GetSampleCount())
}

func TestHandlerRecordsSharedErrorClassifications(t *testing.T) {
	t.Parallel()

	t.Run("HTTPRateLimit", func(t *testing.T) {
		t.Parallel()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		t.Cleanup(upstream.Close)
		rec := &testutil.MockRecorder{}
		provider := &proxyTestProvider{
			MockProvider: &testutil.MockProvider{NameStr: "copilot", URL: upstream.URL, Bridged: []string{"/v1/chat"}},
			typ:          config.ProviderCopilot, authHeader: "Authorization",
		}
		router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, codertestutil.NewFakeSink(t).Logger(), nil, testTracer(t))
		require.NoError(t, err)
		t.Cleanup(router.CloseIdleConnections)
		req := httptest.NewRequest(http.MethodPost, "/copilot/v1/chat", nil)
		req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
		req.Header.Set("Authorization", "Bearer token")
		router.ServeHTTP(httptest.NewRecorder(), req)
		starts := rec.RecordedInterceptions()
		require.Len(t, starts, 1)
		require.Equal(t, recorder.ErrorTypeRateLimited, rec.RecordedInterceptionEnd(starts[0].ID).ErrorType)
	})

	t.Run("Timeout", func(t *testing.T) {
		t.Parallel()
		rec := &testutil.MockRecorder{}
		provider := &proxyTestProvider{
			MockProvider: &testutil.MockProvider{NameStr: "openai", URL: "http://127.0.0.1:1", Bridged: []string{"/v1/chat"}},
			typ:          config.ProviderOpenAI, authHeader: "Authorization",
		}
		router, err := proxy.NewRouter([]aibridge.Provider{provider}, rec, codertestutil.NewFakeSink(t).Logger(), nil, testTracer(t))
		require.NoError(t, err)
		t.Cleanup(router.CloseIdleConnections)
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		defer cancel()
		req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat", bytes.NewReader(nil)).WithContext(ctx)
		req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
		require.PanicsWithValue(t, http.ErrAbortHandler, func() {
			router.ServeHTTP(httptest.NewRecorder(), req)
		})
		starts := rec.RecordedInterceptions()
		require.Len(t, starts, 1)
		require.Equal(t, recorder.ErrorTypeTimeout, rec.RecordedInterceptionEnd(starts[0].ID).ErrorType)
	})
}
