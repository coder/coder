package proxy_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/x/proxy"
	codertestutil "github.com/coder/coder/v2/testutil"
)

func testTracer(t *testing.T) trace.Tracer {
	t.Helper()
	return noop.NewTracerProvider().Tracer("test")
}

type keyPoolProvider struct {
	provider.Provider
	pool *keypool.Pool
}

func (p keyPoolProvider) KeyPool() *keypool.Pool { return p.pool }

func TestNewRouterRegisteredRouteLabels(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	m := metrics.NewMetrics(registry)
	prov := aibridge.NewOpenAIProvider(config.OpenAI{BaseURL: "http://127.0.0.1:1"})
	router, err := proxy.NewRouter([]provider.Provider{prov}, &testutil.MockRecorder{}, slogtest.Make(t, nil), m, testTracer(t))
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)

	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader(`{}`))
	req = req.WithContext(aibridge.AsActor(req.Context(), "actor", nil))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, 1.0, promtestutil.ToFloat64(m.InterceptionCount.WithLabelValues(
		"openai", "", metrics.InterceptionCountStatusFailed, "/v1/chat/completions",
		http.MethodPost, "actor", string(aibridge.ClientUnknown),
	)))
}

func TestNewRouterRequiresRecorderForBridgedRoutes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		provider  provider.Provider
		wantError bool
	}{
		{
			name: "EnabledBridged",
			provider: &testutil.MockProvider{
				NameStr: "openai", URL: "https://openai.example.test",
				Bridged: []string{"/v1/chat/completions"},
			},
			wantError: true,
		},
		{
			name: "PassthroughOnly",
			provider: &testutil.MockProvider{
				NameStr: "openai", URL: "https://openai.example.test",
				Passthrough: []string{"/v1/models"},
			},
		},
		{
			name:     "NoRoutes",
			provider: &testutil.MockProvider{NameStr: "openai", URL: "https://openai.example.test"},
		},
		{
			name: "DisabledBridged",
			provider: &testutil.MockProvider{
				NameStr: "openai", URL: "https://openai.example.test", Disabled: true,
				Bridged: []string{"/v1/chat/completions"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router, err := proxy.NewRouter([]provider.Provider{tc.provider}, nil, slogtest.Make(t, nil), nil, testTracer(t))
			if tc.wantError {
				require.ErrorContains(t, err, "recorder is required for bridged routes")
				require.Nil(t, router)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, router)
			router.CloseIdleConnections()
		})
	}
}

func TestRouterDoesNotWarnForDisabledBedrock(t *testing.T) {
	t.Parallel()

	bedrock := &testutil.MockProvider{
		NameStr: "bedrock", URL: "http://127.0.0.1:1", Disabled: true,
		Bridged: []string{"/v1/messages"},
	}
	sink := codertestutil.NewFakeSink(t)
	router, err := proxy.NewRouter(
		[]provider.Provider{typedProvider{Provider: bedrock, providerType: config.ProviderBedrock}},
		nil, sink.Logger(), nil, testTracer(t),
	)
	require.NoError(t, err)
	require.NotNil(t, router)
	t.Cleanup(router.CloseIdleConnections)
	require.NotContains(t, fmt.Sprint(sink.Entries()), "skipping unsupported Bedrock provider in proxy mode")
}

func TestRouterSkipsBedrock(t *testing.T) {
	t.Parallel()

	bedrock := &testutil.MockProvider{
		NameStr: "bedrock", URL: "http://127.0.0.1:1",
		Bridged: []string{"/v1/messages"}, Passthrough: []string{"/v1/models"},
	}
	bedrockProvider := typedProvider{Provider: bedrock, providerType: config.ProviderBedrock}
	sink := codertestutil.NewFakeSink(t)
	router, err := proxy.NewRouter([]provider.Provider{bedrockProvider}, nil, sink.Logger(), nil, testTracer(t))
	require.NoError(t, err)
	require.Contains(t, fmt.Sprint(sink.Entries()), "skipping unsupported Bedrock provider in proxy mode")

	for _, path := range []string{"/bedrock/v1/messages", "/bedrock/v1/models"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		require.Equal(t, http.StatusNotFound, rec.Code)
	}
}

type typedProvider struct {
	provider.Provider
	providerType string
}

func (p typedProvider) Type() string { return p.providerType }

type invalidFailoverProvider struct {
	*testutil.MockProvider
	config keypool.KeyFailoverConfig
}

func (p *invalidFailoverProvider) KeyPool() *keypool.Pool { return p.config.Pool }
func (p *invalidFailoverProvider) KeyFailoverConfig(slog.Logger) keypool.KeyFailoverConfig {
	return p.config
}

func TestNewRouterValidatesKeyFailoverCallbacks(t *testing.T) {
	t.Parallel()

	pool := testutil.SingleKeyPool(config.ProviderOpenAI, "key")
	for _, tc := range []struct {
		name        string
		config      keypool.KeyFailoverConfig
		errContains string
	}{
		{name: "MissingIsBYOK", config: keypool.KeyFailoverConfig{Pool: pool}, errContains: "IsBYOK callback is required"},
		{name: "MissingInjectAuthKey", config: keypool.KeyFailoverConfig{Pool: pool, IsBYOK: func(*http.Request) bool { return false }}, errContains: "InjectAuthKey callback is required"},
		{name: "MissingBuildResponse", config: keypool.KeyFailoverConfig{Pool: pool, IsBYOK: func(*http.Request) bool { return false }, InjectAuthKey: func(*http.Header, string) {}}, errContains: "BuildKeyPoolResponse callback is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prov := &invalidFailoverProvider{
				MockProvider: &testutil.MockProvider{NameStr: "openai", URL: "https://openai.example.test", Passthrough: []string{"/v1/models"}},
				config:       tc.config,
			}
			router, err := proxy.NewRouter([]provider.Provider{prov}, nil, slogtest.Make(t, nil), nil, testTracer(t))
			require.ErrorContains(t, err, tc.errContains)
			require.Nil(t, router)
		})
	}
}

func TestNewRouterRejectsInvalidBaseURL(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		baseURL     string
		errContains string
	}{
		{name: "Empty"},
		{name: "Relative", baseURL: "/v1"},
		{name: "MissingHost", baseURL: "https:///v1"},
		{name: "UnsupportedScheme", baseURL: "ftp://provider.example.test"},
		{name: "Malformed", baseURL: "http://[::1", errContains: "missing ']' in host"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router, err := proxy.NewRouter([]provider.Provider{&testutil.MockProvider{
				NameStr: "openai",
				URL:     tc.baseURL,
			}}, nil, slogtest.Make(t, nil), nil, testTracer(t))
			if tc.errContains != "" {
				require.ErrorContains(t, err, tc.errContains)
			} else {
				require.ErrorContains(t, err, "absolute HTTP or HTTPS URL with host required")
			}
			require.Nil(t, router)
		})
	}
}

func TestNewRouterValidatesProviders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		providers   []provider.Provider
		errContains string
	}{
		{
			name: "ValidNames",
			providers: []provider.Provider{
				&testutil.MockProvider{NameStr: "openai", URL: "https://openai.example.test"},
				&testutil.MockProvider{NameStr: "anthropic-eu-1", URL: "https://anthropic.example.test"},
			},
		},
		{
			name:        "InvalidName",
			providers:   []provider.Provider{&testutil.MockProvider{NameStr: "OpenAI_1"}},
			errContains: `invalid provider name "OpenAI_1"`,
		},
		{
			name:        "DuplicateName",
			providers:   []provider.Provider{&testutil.MockProvider{NameStr: "openai"}, &testutil.MockProvider{NameStr: "openai"}},
			errContains: `duplicate provider name: "openai"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router, err := proxy.NewRouter(tc.providers, nil, slogtest.Make(t, nil), nil, testTracer(t))
			if tc.errContains != "" {
				require.ErrorContains(t, err, tc.errContains)
				require.Nil(t, router)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, router)
		})
	}
}

// TestRouterDisabledProvider asserts that every path under a disabled
// provider's name serves the 503 sentinel, while a sibling enabled provider
// has no routes yet and falls through to the catch-all.
func TestRouterDisabledProvider(t *testing.T) {
	t.Parallel()

	// Any upstream call would fail loudly: the base URL points nowhere and
	// the mock provider has no interceptor.
	enabled := &testutil.MockProvider{
		NameStr:     "openai",
		URL:         "http://127.0.0.1:1",
		Bridged:     []string{"/v1/chat/completions"},
		Passthrough: []string{"/v1/models"},
	}
	router, err := proxy.NewRouter(
		[]provider.Provider{enabled, provider.NewDisabledStub("disabled-openai", "openai")},
		recorder.NewLogRecorder(slogtest.Make(t, nil), nil), slogtest.Make(t, nil), nil, testTracer(t),
	)
	require.NoError(t, err)

	for _, tc := range []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "DisabledBridgedRoute", path: "/disabled-openai/v1/chat/completions", wantStatus: http.StatusServiceUnavailable, wantBody: routing.ErrorCodeProviderDisabled},
		{name: "DisabledPassthroughRoute", path: "/disabled-openai/v1/models", wantStatus: http.StatusServiceUnavailable, wantBody: routing.ErrorCodeProviderDisabled},
		{name: "DisabledUnknownRoute", path: "/disabled-openai/anything/else", wantStatus: http.StatusServiceUnavailable, wantBody: routing.ErrorCodeProviderDisabled},
		{name: "EnabledBridgedRoute", path: "/openai/v1/chat/completions", wantStatus: http.StatusBadRequest, wantBody: "no actor found"},
		{name: "EnabledPassthroughRoute", path: "/openai/v1/models", wantStatus: http.StatusBadGateway, wantBody: "upstream proxy error"},
		{name: "UnknownProvider", path: "/unknown/v1/models", wantStatus: http.StatusNotFound, wantBody: "route not supported"},
		{name: "Root", path: "/", wantStatus: http.StatusNotFound, wantBody: "route not supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, tc.path, nil))

			assert.Equal(t, tc.wantStatus, resp.Code)
			assert.Contains(t, resp.Body.String(), tc.wantBody)
		})
	}
}

// TestRouterSnapshotsProviders asserts the router routes and reports key
// pools from its own snapshot. Mutating the caller's slice afterwards has
// no effect.
func TestRouterSnapshotsProviders(t *testing.T) {
	t.Parallel()

	pool := testutil.SingleKeyPool(config.ProviderOpenAI, "test-key")
	providers := []provider.Provider{
		keyPoolProvider{Provider: provider.NewDisabledStub("disabled-openai", "openai"), pool: pool},
	}

	router, err := proxy.NewRouter(providers, nil, slogtest.Make(t, nil), nil, testTracer(t))
	require.NoError(t, err)

	// Replace the caller's entry with a provider the router never saw.
	providers[0] = &testutil.MockProvider{NameStr: "swapped"}

	require.NoError(t, promtestutil.CollectAndCompare(keypool.NewStateCollector(router.KeyPools), strings.NewReader(`
# HELP key_pool_state The number of keys currently in each state (state: valid, temporary, permanent).
# TYPE key_pool_state gauge
key_pool_state{provider="openai",state="valid"} 1
key_pool_state{provider="openai",state="temporary"} 0
key_pool_state{provider="openai",state="permanent"} 0
`), "key_pool_state"))

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/disabled-openai/v1/models", nil))
	assert.Equal(t, http.StatusServiceUnavailable, resp.Code)

	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/swapped/v1/models", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}

// Disabled providers return 503 without reading the body, even if it is oversized.
func TestRouterDisabledProviderOversizedBody(t *testing.T) {
	t.Parallel()

	router, err := proxy.NewRouter(
		[]provider.Provider{provider.NewDisabledStub("disabled-openai", "openai")},
		nil, slogtest.Make(t, nil), nil, testTracer(t),
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/disabled-openai/v1/chat/completions", strings.NewReader("x"))
	req.ContentLength = routing.MaxRequestBodyBytes + 1

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), routing.ErrorCodeProviderDisabled)
}
