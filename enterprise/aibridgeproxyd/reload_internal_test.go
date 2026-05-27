package aibridgeproxyd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

func TestServerReloadSwapsProviderRouter(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	providers := []ProviderRoute{{Name: "old", BaseURL: "https://old.example.com/"}}
	srv := &Server{
		ctx:          ctx,
		logger:       slogtest.Make(t, nil),
		allowedPorts: []string{"443"},
		refreshProviders: func(context.Context) ([]ProviderRoute, error) {
			return providers, nil
		},
	}
	srv.providerRouter.Store(emptyProviderRouter)

	require.NoError(t, srv.Reload(ctx))
	assert.Equal(t, "old", srv.loadProviderRouter().providerFromHost("old.example.com"))
	assert.Empty(t, srv.loadProviderRouter().providerFromHost("new.example.com"))

	providers = []ProviderRoute{{Name: "new", BaseURL: "https://new.example.com/"}}
	require.NoError(t, srv.Reload(ctx))

	router := srv.loadProviderRouter()
	assert.Empty(t, router.providerFromHost("old.example.com"))
	assert.Equal(t, "new", router.providerFromHost("new.example.com"))
	assert.Equal(t, []string{"new.example.com:443"}, router.mitmHosts)
}

func TestServerReloadPreservesProviderRouterOnRefreshError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	refreshErr := xerrors.New("refresh failed")
	providers := []ProviderRoute{{Name: "old", BaseURL: "https://old.example.com/"}}
	failRefresh := false
	srv := &Server{
		ctx:          ctx,
		logger:       slogtest.Make(t, nil),
		allowedPorts: []string{"443"},
		refreshProviders: func(context.Context) ([]ProviderRoute, error) {
			if failRefresh {
				return nil, refreshErr
			}
			return providers, nil
		},
	}
	srv.providerRouter.Store(emptyProviderRouter)

	require.NoError(t, srv.Reload(ctx))
	before := srv.loadProviderRouter()
	assert.Equal(t, "old", before.providerFromHost("old.example.com"))

	failRefresh = true
	require.ErrorIs(t, srv.Reload(ctx), refreshErr)

	after := srv.loadProviderRouter()
	assert.Same(t, before, after)
	assert.Equal(t, "old", after.providerFromHost("old.example.com"))
	assert.Equal(t, []string{"old.example.com:443"}, after.mitmHosts)
}

// TestBuildProviderRouter covers the host-and-routing derivation that
// Reload feeds into the providerRouter.
func TestBuildProviderRouter(t *testing.T) {
	t.Parallel()

	t.Run("ExtractsHostnames", func(t *testing.T) {
		t.Parallel()

		providers := []ProviderRoute{
			{Name: "openai", BaseURL: "https://api.openai.com/v1/"},
			{Name: "anthropic", BaseURL: "https://api.anthropic.com/"},
			{Name: "custom", BaseURL: "https://custom-llm.example.com:8443/api"},
		}

		router, err := buildProviderRouter(testutil.Context(t, testutil.WaitShort), slogtest.Make(t, nil), providers, []string{"443"})
		require.NoError(t, err)

		assert.Equal(t, "openai", router.providerFromHost("api.openai.com"))
		assert.Equal(t, "anthropic", router.providerFromHost("api.anthropic.com"))
		assert.Equal(t, "custom", router.providerFromHost("custom-llm.example.com"))
		assert.Empty(t, router.providerFromHost("unknown.com"))

		assert.Contains(t, router.mitmHosts, "api.openai.com:443")
		assert.Contains(t, router.mitmHosts, "api.anthropic.com:443")
	})

	t.Run("DeduplicatesSameHost", func(t *testing.T) {
		t.Parallel()

		providers := []ProviderRoute{
			{Name: "first", BaseURL: "https://api.example.com/v1"},
			{Name: "second", BaseURL: "https://api.example.com/v2"},
		}

		router, err := buildProviderRouter(testutil.Context(t, testutil.WaitShort), slogtest.Make(t, nil), providers, []string{"443"})
		require.NoError(t, err)

		// First provider wins on duplicate host.
		assert.Equal(t, "first", router.providerFromHost("api.example.com"))
	})

	t.Run("CaseInsensitive", func(t *testing.T) {
		t.Parallel()

		providers := []ProviderRoute{
			{Name: "provider", BaseURL: "https://API.Example.COM/v1"},
		}

		router, err := buildProviderRouter(testutil.Context(t, testutil.WaitShort), slogtest.Make(t, nil), providers, []string{"443"})
		require.NoError(t, err)

		assert.Equal(t, "provider", router.providerFromHost("API.Example.COM"))
		assert.Equal(t, "provider", router.providerFromHost("api.example.com"))
	})

	t.Run("SkipsEmptyOrMalformedBaseURL", func(t *testing.T) {
		t.Parallel()

		providers := []ProviderRoute{
			{Name: "no-url"},
			{Name: "scheme-only", BaseURL: "https://"},
			{Name: "good", BaseURL: "https://api.good.example.com/"},
		}

		router, err := buildProviderRouter(testutil.Context(t, testutil.WaitShort), slogtest.Make(t, nil), providers, []string{"443"})
		require.NoError(t, err)

		assert.Equal(t, "good", router.providerFromHost("api.good.example.com"))
		assert.Empty(t, router.providerFromHost("scheme-only"))
		assert.Equal(t, []string{"api.good.example.com:443"}, router.mitmHosts)
	})
}
