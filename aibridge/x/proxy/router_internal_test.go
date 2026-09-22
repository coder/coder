package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	codertestutil "github.com/coder/coder/v2/testutil"
)

type typedProvider struct {
	provider.Provider
	providerType string
}

func (p typedProvider) Type() string { return p.providerType }

func TestBuildRouterAssembly(t *testing.T) {
	t.Parallel()

	t.Run("RequiresRecorderForBridgedRoutes", func(t *testing.T) {
		t.Parallel()
		prov := &testutil.MockProvider{
			NameStr: "openai", URL: "https://openai.example.test",
			Bridged: []string{"/v1/chat/completions"},
		}
		router, err := buildRouter([]provider.Provider{prov}, nil, slogtest.Make(t, nil), nil, nil)
		require.ErrorContains(t, err, "recorder is required for bridged routes")
		require.Nil(t, router)
	})

	t.Run("OneTransportPerProvider", func(t *testing.T) {
		t.Parallel()
		prov := &recordingProvider{
			MockProvider: &testutil.MockProvider{
				NameStr: "openai", URL: "http://127.0.0.1:1",
				Bridged:     []string{"/v1/chat", "/v1/responses"},
				Passthrough: []string{"/v1/models", "/v1/files"},
			},
			breaker: &config.CircuitBreaker{FailureThreshold: 1},
		}
		router := newTestRouter(t, []provider.Provider{prov}, &testutil.MockRecorder{}, slogtest.Make(t, nil), nil)
		require.Len(t, router.transports, 1)

		bridged, _ := router.mux.Handler(httptest.NewRequest(http.MethodPost, "/openai/v1/chat", nil))
		passthrough, _ := router.mux.Handler(httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil))
		bridgedHandler, ok := bridged.(*forwardingHandler)
		require.True(t, ok)
		passthroughHandler, ok := passthrough.(*forwardingHandler)
		require.True(t, ok)
		require.NotSame(t, bridgedHandler, passthroughHandler)
		require.Equal(t, bridgedHandler.transport, passthroughHandler.transport)
		require.True(t, bridgedHandler.record)
		require.False(t, passthroughHandler.record)
		require.NotNil(t, bridgedHandler.breaker)
		require.Nil(t, passthroughHandler.breaker)
		require.Equal(t, "/v1/chat", bridgedHandler.metricRoute)
		require.Equal(t, "/v1/models", passthroughHandler.metricRoute)
	})

	t.Run("PropagatesHandlerValidation", func(t *testing.T) {
		t.Parallel()
		prov := &testutil.MockProvider{NameStr: "openai", URL: "/relative", Passthrough: []string{"/v1/models"}}
		router, err := buildRouter([]provider.Provider{prov}, nil, slogtest.Make(t, nil), nil, nil)
		require.ErrorContains(t, err, "absolute HTTP or HTTPS URL")
		require.Nil(t, router)
	})

	t.Run("SnapshotsProviders", func(t *testing.T) {
		t.Parallel()
		providers := []provider.Provider{provider.NewDisabledStub("disabled-openai", config.ProviderOpenAI)}
		router := newTestRouter(t, providers, nil, slogtest.Make(t, nil), nil)
		providers[0] = &testutil.MockProvider{NameStr: "swapped", URL: "https://provider.example.test"}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/disabled-openai/v1/models", nil))
		require.Equal(t, http.StatusServiceUnavailable, response.Code)
		response = httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/swapped/v1/models", nil))
		require.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("SkipsBedrock", func(t *testing.T) {
		t.Parallel()
		sink := codertestutil.NewFakeSink(t)
		prov := typedProvider{
			Provider: &testutil.MockProvider{
				NameStr: "bedrock", URL: "http://127.0.0.1:1",
				Bridged: []string{"/v1/messages"}, Passthrough: []string{"/v1/models"},
			},
			providerType: config.ProviderBedrock,
		}
		router := newTestRouter(t, []provider.Provider{prov}, nil, sink.Logger(), nil)
		require.Empty(t, router.transports)
		require.Contains(t, fmt.Sprint(sink.Entries()), "skipping unsupported Bedrock provider in proxy mode")
		for _, path := range []string{"/bedrock/v1/messages", "/bedrock/v1/models"} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
			require.Equal(t, http.StatusNotFound, response.Code)
		}
	})

	t.Run("DisabledBedrockDoesNotWarn", func(t *testing.T) {
		t.Parallel()
		sink := codertestutil.NewFakeSink(t)
		prov := typedProvider{
			Provider:     &testutil.MockProvider{NameStr: "bedrock", Disabled: true, Bridged: []string{"/v1/messages"}},
			providerType: config.ProviderBedrock,
		}
		router := newTestRouter(t, []provider.Provider{prov}, nil, sink.Logger(), nil)
		require.NotNil(t, router)
		require.NotContains(t, fmt.Sprint(sink.Entries()), "skipping unsupported Bedrock provider in proxy mode")
	})
}

func newTestRouter(t *testing.T, providers []provider.Provider, rec recorder.Recorder, logger slog.Logger, m *metrics.Metrics) *Router {
	t.Helper()
	router, err := buildRouter(providers, rec, logger, m, noop.NewTracerProvider().Tracer(t.Name()))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)
	return router
}
