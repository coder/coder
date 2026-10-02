package proxy_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	aibtestutil "github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/x/proxy"
	"github.com/coder/coder/v2/testutil"
)

type keyPoolProvider struct {
	provider.Provider
	pool *keypool.Pool
}

func (p keyPoolProvider) KeyPool() *keypool.Pool { return p.pool }

func newRouterGate(t *testing.T) *aibridge.InflightGate {
	t.Helper()
	gate := aibridge.NewInflightGate(slogtest.Make(t, nil))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), testutil.IntervalSlow)
		defer cancel()
		defer gate.Close()
		require.NoError(t, gate.Shutdown(ctx))
	})
	return gate
}

func TestNewRouterValidatesProviders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		providers   []provider.Provider
		errContains string
	}{
		{
			name:      "ValidNames",
			providers: []provider.Provider{&aibtestutil.MockProvider{NameStr: "openai"}, &aibtestutil.MockProvider{NameStr: "anthropic-eu-1"}},
		},
		{
			name:        "InvalidName",
			providers:   []provider.Provider{&aibtestutil.MockProvider{NameStr: "OpenAI_1"}},
			errContains: `invalid provider name "OpenAI_1"`,
		},
		{
			name:        "DuplicateName",
			providers:   []provider.Provider{&aibtestutil.MockProvider{NameStr: "openai"}, &aibtestutil.MockProvider{NameStr: "openai"}},
			errContains: `duplicate provider name: "openai"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router, err := proxy.NewRouter(tc.providers, slogtest.Make(t, nil), nil, noop.NewTracerProvider().Tracer(t.Name()), newRouterGate(t), nil)
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

//nolint:paralleltest,tparallel // Sequential subtests verify connection reuse.
func TestRouterRoutes(t *testing.T) {
	t.Parallel()

	var calls, connections atomic.Int32
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, r.URL.Path)
	}))
	upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	upstream.Start()
	t.Cleanup(upstream.Close)
	enabled := &aibtestutil.MockProvider{
		NameStr:     "openai",
		URL:         upstream.URL,
		Bridged:     []string{"/v1/chat/completions", "/v1/models/bridged"},
		Passthrough: []string{"/v1/models", "/v1/models/", "/v1/tokens"},
	}
	m := metrics.NewMetrics(prometheus.NewRegistry())
	router, err := proxy.NewRouter(
		[]provider.Provider{enabled, provider.NewDisabledStub("disabled-openai", "openai")},
		slogtest.Make(t, nil), m, noop.NewTracerProvider().Tracer(t.Name()), newRouterGate(t), nil,
	)
	require.NoError(t, err)

	const disabledBody = routing.ErrorCodeProviderDisabled + ": AI provider \"disabled-openai\" is disabled\n"
	var passthroughPaths []string
	for _, tc := range []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "DisabledProvider/BridgedRoute",
			path:       "/disabled-openai/v1/chat/completions",
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   disabledBody,
		},
		{
			name:       "DisabledProvider/PassthroughRoute",
			path:       "/disabled-openai/v1/models",
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   disabledBody,
		},
		{
			name:       "DisabledProvider/UnknownRoute",
			path:       "/disabled-openai/anything/else",
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   disabledBody,
		},
		{
			name:       "BridgedRoute",
			path:       "/openai/v1/chat/completions",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
		},
		{
			name:       "BridgedRouteOverridesPassthrough",
			path:       "/openai/v1/models/bridged",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
		},
		{
			name:       "PassthroughRoute/Models",
			path:       "/openai/v1/models",
			wantStatus: http.StatusOK,
			wantBody:   "/v1/models",
		},
		{
			name:       "PassthroughRoute/Tokens",
			path:       "/openai/v1/tokens",
			wantStatus: http.StatusOK,
			wantBody:   "/v1/tokens",
		},
		{
			name:       "UnknownRoute/EnabledProvider",
			path:       "/openai/admin",
			wantStatus: http.StatusNotFound,
			wantBody:   "route not supported: GET /openai/admin\n",
		},
		{
			name:       "UnknownRoute/UnknownProvider",
			path:       "/unknown/v1/models",
			wantStatus: http.StatusNotFound,
			wantBody:   "route not supported: GET /unknown/v1/models\n",
		},
		{
			name:       "UnknownRoute/Root",
			path:       "/",
			wantStatus: http.StatusNotFound,
			wantBody:   "route not supported: GET /\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantStatus == http.StatusOK {
				passthroughPaths = append(passthroughPaths, tc.wantBody)
			}
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, tc.path, nil))
			assert.Equal(t, tc.wantStatus, resp.Code, "path: %s", tc.path)
			assert.Equal(t, tc.wantBody, resp.Body.String(), "path: %s", tc.path)
		})
	}

	require.EqualValues(t, len(passthroughPaths), calls.Load(), "only passthrough routes must reach upstream")
	require.EqualValues(t, min(len(passthroughPaths), 1), connections.Load(), "passthrough routes must share the provider transport")
	require.Equal(t, len(passthroughPaths), promtestutil.CollectAndCount(m.PassthroughCount))
	for _, path := range passthroughPaths {
		require.Equal(t, float64(1), promtestutil.ToFloat64(m.PassthroughCount.WithLabelValues("openai", path, http.MethodGet)), "path: %s", path)
	}
}

// TestRouterRefusesAfterGateShutdown asserts every route kind is admitted
// through the supplied gate, so none is served once it shuts down.
func TestRouterRefusesAfterGateShutdown(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("refused requests must not reach upstream")
	}))
	t.Cleanup(upstream.Close)
	gate := newRouterGate(t)
	router, err := proxy.NewRouter(
		[]provider.Provider{
			&aibtestutil.MockProvider{
				NameStr:     "openai",
				URL:         upstream.URL,
				Bridged:     []string{"/v1/chat/completions"},
				Passthrough: []string{"/v1/models"},
			},
			provider.NewDisabledStub("disabled-openai", "openai"),
		},
		slogtest.Make(t, nil), nil, noop.NewTracerProvider().Tracer(t.Name()), gate, nil,
	)
	require.NoError(t, err)
	require.NoError(t, gate.Shutdown(testutil.Context(t, testutil.WaitShort)))

	for _, tc := range []struct {
		name string
		path string
	}{
		{
			name: "BridgedRoute",
			path: "/openai/v1/chat/completions",
		},
		{
			name: "PassthroughRoute",
			path: "/openai/v1/models",
		},
		{
			name: "DisabledProvider",
			path: "/disabled-openai/v1/models",
		},
		{
			name: "UnknownRoute",
			path: "/unknown/v1/models",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, tc.path, nil))
			assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
			assert.Equal(t, "AI Gateway is shutting down\n", resp.Body.String())
		})
	}
}

// TestRouterSnapshotsProviders asserts the router routes and reports key
// pools from its own snapshot. Mutating the caller's slice afterwards has
// no effect.
func TestRouterSnapshotsProviders(t *testing.T) {
	t.Parallel()

	pool := aibtestutil.SingleKeyPool(config.ProviderOpenAI, "test-key")
	providers := []provider.Provider{
		keyPoolProvider{Provider: provider.NewDisabledStub("disabled-openai", "openai"), pool: pool},
	}

	router, err := proxy.NewRouter(providers, slogtest.Make(t, nil), nil, noop.NewTracerProvider().Tracer(t.Name()), newRouterGate(t), nil)
	require.NoError(t, err)

	// Replace the caller's entry with a provider the router never saw.
	providers[0] = &aibtestutil.MockProvider{NameStr: "swapped"}

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
		slogtest.Make(t, nil), nil, noop.NewTracerProvider().Tracer(t.Name()), newRouterGate(t), nil,
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/disabled-openai/v1/chat/completions", strings.NewReader("x"))
	req.ContentLength = routing.MaxRequestBodyBytes + 1

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), routing.ErrorCodeProviderDisabled)
}
