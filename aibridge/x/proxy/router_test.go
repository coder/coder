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

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	aibtestutil "github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/metrics"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
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

			router, err := proxy.NewRouter(t.Context(), tc.providers, slogtest.Make(t, nil), nil, noop.NewTracerProvider().Tracer(t.Name()), newRouterGate(t), nil)
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
		Bridged:     []string{"/bridged/exact/path", "/bridged/whole/subtree/", "/passthrough/whole/subtree/bridged"},
		Passthrough: []string{"/passthrough/exact/path", "/passthrough/whole/subtree/"},
	}
	m := metrics.NewMetrics(prometheus.NewRegistry())
	router, err := proxy.NewRouter(
		t.Context(), []provider.Provider{enabled, provider.NewDisabledStub("disabled-openai", "openai")},
		slogtest.Make(t, nil), m, noop.NewTracerProvider().Tracer(t.Name()), newRouterGate(t), &struct{ recorder.Recorder }{},
	)
	require.NoError(t, err)

	const bridgedBody = "no actor found\n"
	const disabledBody = routing.ErrorCodeProviderDisabled + ": AI provider \"disabled-openai\" is disabled\n"
	var passthroughPaths []string
	for _, tc := range []struct {
		name       string
		path       string
		noActor    bool
		wantStatus int
		wantBody   string
	}{
		{
			name:       "DisabledProvider_Bridged_ExactPath",
			path:       "/disabled-openai/bridged/exact/path",
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   disabledBody,
		},
		{
			name:       "DisabledProvider_Bridged_SubtreePath",
			path:       "/disabled-openai/bridged/whole/subtree/nested",
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   disabledBody,
		},
		{
			name:       "DisabledProvider_Passthrough_ExactPath",
			path:       "/disabled-openai/passthrough/exact/path",
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   disabledBody,
		},
		{
			name:       "DisabledProvider_Passthrough_SubtreePath",
			path:       "/disabled-openai/passthrough/whole/subtree/nested",
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   disabledBody,
		},
		{
			name:       "DisabledProvider_UnknownPath",
			path:       "/disabled-openai/unknown/path",
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   disabledBody,
		},
		{
			name:       "EnabledProvider_Bridged_ExactPath",
			path:       "/openai/bridged/exact/path",
			noActor:    true,
			wantStatus: http.StatusBadRequest,
			wantBody:   bridgedBody,
		},
		{
			name:       "EnabledProvider_Bridged_SubtreePath",
			path:       "/openai/bridged/whole/subtree/nested",
			noActor:    true,
			wantStatus: http.StatusBadRequest,
			wantBody:   bridgedBody,
		},
		{
			name:       "BridgedRouteWithoutActor",
			path:       "/openai/bridged/exact/path",
			noActor:    true,
			wantStatus: http.StatusBadRequest,
			wantBody:   "no actor found\n",
		},
		{
			name:       "EnabledProvider_Bridged_ExactPathOverridesPassthroughSubtree",
			path:       "/openai/passthrough/whole/subtree/bridged",
			noActor:    true,
			wantStatus: http.StatusBadRequest,
			wantBody:   bridgedBody,
		},
		{
			name:       "EnabledProvider_Passthrough_ExactPath",
			path:       "/openai/passthrough/exact/path",
			wantStatus: http.StatusOK,
			wantBody:   "/passthrough/exact/path",
		},
		{
			name:       "EnabledProvider_Passthrough_SubtreePath",
			path:       "/openai/passthrough/whole/subtree/nested",
			wantStatus: http.StatusOK,
			wantBody:   "/passthrough/whole/subtree/nested",
		},
		{
			name:       "EnabledProvider_UnknownPath",
			path:       "/openai/unknown/path",
			wantStatus: http.StatusNotFound,
			wantBody:   "route not supported: GET /openai/unknown/path\n",
		},
		{
			name:       "UnknownProvider",
			path:       "/unknown/path",
			wantStatus: http.StatusNotFound,
			wantBody:   "route not supported: GET /unknown/path\n",
		},
		{
			name:       "EncodedTraversal",
			path:       "/openai/v1/models/%2e%2e/files",
			wantStatus: http.StatusBadRequest,
			wantBody:   routing.InvalidPathMessage + "\n",
		},
		{
			name:       "DisabledEncodedTraversal",
			path:       "/disabled-openai/v1/models/..%2F..%2Fadmin",
			wantStatus: http.StatusBadRequest,
			wantBody:   routing.InvalidPathMessage + "\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantStatus == http.StatusOK {
				passthroughPaths = append(passthroughPaths, tc.wantBody)
			}
			req := httptest.NewRequest(http.MethodGet, tc.path, nil).WithContext(t.Context())
			if !tc.noActor {
				req = req.WithContext(aibcontext.AsActor(req.Context(), aibcontext.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()}))
			}
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, req)
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
		t.Context(), []provider.Provider{
			&aibtestutil.MockProvider{
				NameStr:     "openai",
				URL:         upstream.URL,
				Bridged:     []string{"/v1/chat/completions"},
				Passthrough: []string{"/v1/models"},
			},
			provider.NewDisabledStub("disabled-openai", "openai"),
		},
		slogtest.Make(t, nil), nil, noop.NewTracerProvider().Tracer(t.Name()), gate, &struct{ recorder.Recorder }{},
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

			ctx := aibcontext.AsActor(t.Context(), aibcontext.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()})
			resp := httptest.NewRecorder()
			router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, tc.path, nil).WithContext(ctx))
			assert.Equal(t, http.StatusServiceUnavailable, resp.Code)
			assert.Equal(t, "AI Gateway is shutting down\n", resp.Body.String())
		})
	}
}

func TestRouterRecordedForwardingIsolation(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, r.Body)
	}))
	t.Cleanup(upstream.Close)
	rec := &aibtestutil.MockRecorder{}
	router, err := proxy.NewRouter(t.Context(), []provider.Provider{provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL})}, slogtest.Make(t, nil), nil, noop.NewTracerProvider().Tracer(t.Name()), newRouterGate(t), rec)
	require.NoError(t, err)
	t.Cleanup(router.CloseIdleConnections)
	t.Cleanup(func() {
		rec.VerifyAllInterceptionsEnded(t)
		require.Empty(t, rec.RecordedTokenUsages())
		require.Empty(t, rec.RecordedPromptUsages())
		require.Empty(t, rec.RecordedToolUsages())
		require.Empty(t, rec.RecordedModelThoughts())
	})
	for _, route := range []string{"/chat/completions", "/responses"} {
		t.Run(route, func(t *testing.T) {
			t.Parallel()
			actor := aibcontext.Actor{ID: uuid.New(), APIKeyID: uuid.NewString(), Username: t.Name()}
			req := httptest.NewRequest(http.MethodPost, "/openai/v1"+route, strings.NewReader(actor.Username)).WithContext(aibcontext.AsActor(t.Context(), actor))
			req.Header.Set("Authorization", "Bearer user-key")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, actor.Username, response.Body.String())
			var matches []*recorder.InterceptionRecord
			for _, record := range rec.RecordedInterceptions() {
				if record.InitiatorID == actor.ID.String() {
					matches = append(matches, record)
				}
			}
			require.Len(t, matches, 1)
			start := matches[0]
			require.Equal(t, recorder.Metadata{"Username": actor.Username}, start.Metadata)
			require.Empty(t, start.Model)
			end := rec.RecordedInterceptionEnd(start.ID)
			require.NotNil(t, end)
			require.Empty(t, end.ErrorType)
		})
	}
}

func TestRouterBridgedRejectsRequest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		actor    bool
		shutdown bool
		status   int
		body     string
	}{
		{
			name:   "Rejected",
			status: http.StatusBadRequest,
			body:   "no actor found\n",
		},
		{
			name:     "AfterShutdown",
			actor:    true,
			shutdown: true,
			status:   http.StatusServiceUnavailable,
			body:     "AI Gateway is shutting down\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
			t.Cleanup(upstream.Close)
			gate := newRouterGate(t)
			rec := &aibtestutil.MockRecorder{}
			router, err := proxy.NewRouter(t.Context(), []provider.Provider{provider.NewOpenAI(config.OpenAI{BaseURL: upstream.URL})}, slogtest.Make(t, nil), nil, noop.NewTracerProvider().Tracer(t.Name()), gate, rec)
			require.NoError(t, err)
			if tc.shutdown {
				require.NoError(t, gate.Shutdown(t.Context()))
			}
			req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
			if tc.actor {
				req = req.WithContext(aibcontext.AsActor(t.Context(), aibcontext.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()}))
			}
			req.Header.Set("Authorization", "Bearer user-key")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.NoError(t, gate.Shutdown(testutil.Context(t, testutil.WaitLong)))
			require.Equal(t, tc.status, response.Code)
			require.Equal(t, tc.body, response.Body.String())
			require.Zero(t, calls.Load(), "rejected requests must not reach upstream")
			require.Empty(t, rec.RecordedInterceptions())
			rec.VerifyAllInterceptionsEnded(t)
		})
	}
}

func TestRouterBedrockBridgedRemainsDisabled(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		authorization string
	}{
		{
			name: "NoAuth",
		},
		{
			name:          "BYOK",
			authorization: "Bearer user-key",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
			t.Cleanup(upstream.Close)
			prov, err := provider.NewBedrock(t.Context(), config.Anthropic{BaseURL: upstream.URL}, config.AWSBedrock{
				Region: "us-east-1", AccessKey: "test-key", AccessKeySecret: "test-secret",
				Model: "test-model", SmallFastModel: "test-small-model",
			})
			require.NoError(t, err)
			rec := &aibtestutil.MockRecorder{}
			router, err := proxy.NewRouter(t.Context(), []provider.Provider{prov}, slogtest.Make(t, nil), nil, noop.NewTracerProvider().Tracer(t.Name()), newRouterGate(t), rec)
			require.NoError(t, err)
			ctx := aibcontext.AsActor(t.Context(), aibcontext.Actor{ID: uuid.New(), APIKeyID: uuid.NewString()})
			for _, route := range prov.BridgedRoutes() {
				req := httptest.NewRequest(http.MethodPost, prov.RoutePrefix()+route, nil).WithContext(ctx)
				if tc.authorization != "" {
					req.Header.Set("Authorization", tc.authorization)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, req)
				require.Equal(t, http.StatusNotFound, response.Code)
				require.Equal(t, "404 page not found\n", response.Body.String())
				require.Empty(t, rec.RecordedInterceptions())
				rec.VerifyAllInterceptionsEnded(t)
			}
			require.Zero(t, calls.Load(), "Bedrock forwarding is not supported")
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

	router, err := proxy.NewRouter(t.Context(), providers, slogtest.Make(t, nil), nil, noop.NewTracerProvider().Tracer(t.Name()), newRouterGate(t), nil)
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
		t.Context(), []provider.Provider{provider.NewDisabledStub("disabled-openai", "openai")},
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
