package aibridge_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
)

// poolProvider decorates a provider with a key pool, which
// [testutil.MockProvider] never has.
type poolProvider struct {
	aibridge.Provider
	pool *keypool.Pool
}

func (p poolProvider) KeyPool() *keypool.Pool { return p.pool }

func TestNewProxyRouterValidatesProviders(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		providers   []aibridge.Provider
		errContains string
	}{
		{
			name:      "ValidNames",
			providers: []aibridge.Provider{&testutil.MockProvider{NameStr: "openai"}, &testutil.MockProvider{NameStr: "anthropic-eu-1"}},
		},
		{
			name:        "InvalidName",
			providers:   []aibridge.Provider{&testutil.MockProvider{NameStr: "OpenAI_1"}},
			errContains: `invalid provider name "OpenAI_1"`,
		},
		{
			name:        "DuplicateName",
			providers:   []aibridge.Provider{&testutil.MockProvider{NameStr: "openai"}, &testutil.MockProvider{NameStr: "openai"}},
			errContains: `duplicate provider name: "openai"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			router, err := aibridge.NewProxyRouter(tc.providers, slogtest.Make(t, nil))
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

// TestProxyRouterDisabledProvider asserts that every path under a disabled
// provider's name serves the 503 sentinel, while a sibling enabled provider
// has no routes yet and falls through to the catch-all.
func TestProxyRouterDisabledProvider(t *testing.T) {
	t.Parallel()

	// Any upstream call would fail loudly: the base URL points nowhere and
	// the mock provider has no interceptor.
	enabled := &testutil.MockProvider{
		NameStr:     "openai",
		URL:         "http://127.0.0.1:1",
		Bridged:     []string{"/v1/chat/completions"},
		Passthrough: []string{"/v1/models"},
	}
	router, err := aibridge.NewProxyRouter(
		[]aibridge.Provider{enabled, aibridge.NewDisabledProviderStub("disabled-openai", "openai")},
		slogtest.Make(t, nil),
	)
	require.NoError(t, err)

	for _, tc := range []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "DisabledBridgedRoute", path: "/disabled-openai/v1/chat/completions", wantStatus: http.StatusServiceUnavailable, wantBody: aibridge.ErrorCodeProviderDisabled},
		{name: "DisabledPassthroughRoute", path: "/disabled-openai/v1/models", wantStatus: http.StatusServiceUnavailable, wantBody: aibridge.ErrorCodeProviderDisabled},
		{name: "DisabledUnknownRoute", path: "/disabled-openai/anything/else", wantStatus: http.StatusServiceUnavailable, wantBody: aibridge.ErrorCodeProviderDisabled},
		{name: "EnabledBridgedRoute", path: "/openai/v1/chat/completions", wantStatus: http.StatusNotFound, wantBody: "route not supported"},
		{name: "EnabledPassthroughRoute", path: "/openai/v1/models", wantStatus: http.StatusNotFound, wantBody: "route not supported"},
		{name: "UnknownProvider", path: "/nope/v1/models", wantStatus: http.StatusNotFound, wantBody: "route not supported"},
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

// TestProxyRouterSnapshotsProviders asserts the router routes and reports key
// pools from its own snapshot, so mutating the caller's slice afterwards has
// no effect.
func TestProxyRouterSnapshotsProviders(t *testing.T) {
	t.Parallel()

	pool := testutil.SingleKeyPool(config.ProviderOpenAI, "test-key")
	providers := []aibridge.Provider{
		poolProvider{Provider: aibridge.NewDisabledProviderStub("disabled-openai", "openai"), pool: pool},
	}

	router, err := aibridge.NewProxyRouter(providers, slogtest.Make(t, nil))
	require.NoError(t, err)

	// Replace the caller's entry with a provider the router never saw.
	providers[0] = &testutil.MockProvider{NameStr: "swapped"}

	require.Equal(t, []*keypool.Pool{pool}, router.KeyPools())

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/disabled-openai/v1/models", nil))
	assert.Equal(t, http.StatusServiceUnavailable, resp.Code)

	resp = httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodPost, "/swapped/v1/models", nil))
	assert.Equal(t, http.StatusNotFound, resp.Code)
}
