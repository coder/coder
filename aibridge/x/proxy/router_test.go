package proxy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/routing"
	"github.com/coder/coder/v2/aibridge/x/proxy"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
)

type keyPoolProvider struct {
	provider.Provider
	pool *keypool.Pool
}

func (p keyPoolProvider) KeyPool() *keypool.Pool { return p.pool }

func TestNewRouterValidatesProviders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		providers   []provider.Provider
		errContains string
	}{
		{
			name:      "ValidNames",
			providers: []provider.Provider{&testutil.MockProvider{NameStr: "openai"}, &testutil.MockProvider{NameStr: "anthropic-eu-1"}},
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

			router, err := proxy.NewRouter(tc.providers, slogtest.Make(t, nil))
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
		slogtest.Make(t, nil),
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
		{name: "EnabledBridgedRoute", path: "/openai/v1/chat/completions", wantStatus: http.StatusNotFound, wantBody: "route not supported"},
		{name: "EnabledPassthroughRoute", path: "/openai/v1/models", wantStatus: http.StatusNotFound, wantBody: "route not supported"},
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

	router, err := proxy.NewRouter(providers, slogtest.Make(t, nil))
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
		slogtest.Make(t, nil),
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/disabled-openai/v1/chat/completions", strings.NewReader("x"))
	req.ContentLength = routing.MaxRequestBodyBytes + 1

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), routing.ErrorCodeProviderDisabled)
}

// TestRouterOpensNoResponsesWebSocket asserts that proxy mode, which does
// not serve Responses WebSocket mode, never takes a socket lease for an
// upgrade, even for a user with the experiment on.
func TestRouterOpensNoResponsesWebSocket(t *testing.T) {
	t.Parallel()

	router, err := proxy.NewRouter([]provider.Provider{provider.NewOpenAI(config.OpenAI{BaseURL: "http://127.0.0.1:1"})}, slogtest.Make(t, nil))
	require.NoError(t, err)

	acquired := false
	ctx := agplaibridge.WithResponsesWebSocketEnabled(context.Background(), true)
	ctx = aibcontext.AsActor(ctx, "actor", "", nil)
	ctx = aibcontext.WithSocketAcquirer(ctx, func(context.Context, string, string) (aibcontext.SocketLease, error) {
		acquired = true
		return nil, xerrors.New("must not acquire")
	})
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/openai/v1/responses", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusNotFound, resp.Code)
	assert.False(t, acquired)
}
