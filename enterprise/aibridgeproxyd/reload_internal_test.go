package aibridgeproxyd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/testutil"
)

func enabledProvider(name, host string) ReloadedProvider {
	return ReloadedProvider{
		ProviderOutcome: aibridged.ProviderOutcome{
			Name:   name,
			Type:   "openai",
			Status: aibridged.ProviderStatusEnabled,
		},
		Host: host,
	}
}

func TestServerReloadSwapsProviderRouter(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	reload := ProviderReload{Providers: []ReloadedProvider{enabledProvider("old", "old.example.com")}}
	srv := &Server{
		ctx:          ctx,
		logger:       slogtest.Make(t, nil),
		allowedPorts: []string{"443"},
		refreshProviders: func(context.Context) (ProviderReload, error) {
			return reload, nil
		},
	}
	srv.providerRouter.Store(emptyProviderRouter)

	require.NoError(t, srv.Reload(ctx))
	assert.Equal(t, routedProvider{name: "old", providerType: "openai"}, srv.loadProviderRouter().providerFromHost("old.example.com"))
	assert.Empty(t, srv.loadProviderRouter().providerFromHost("new.example.com"))

	reload = ProviderReload{Providers: []ReloadedProvider{enabledProvider("new", "new.example.com")}}
	require.NoError(t, srv.Reload(ctx))

	router := srv.loadProviderRouter()
	assert.Empty(t, router.providerFromHost("old.example.com"))
	assert.Equal(t, routedProvider{name: "new", providerType: "openai"}, router.providerFromHost("new.example.com"))
	assert.Equal(t, []string{"new.example.com:443"}, router.mitmHosts)
}

func TestServerReloadPreservesProviderRouterOnRefreshError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	refreshErr := xerrors.New("refresh failed")
	reload := ProviderReload{Providers: []ReloadedProvider{enabledProvider("old", "old.example.com")}}
	failRefresh := false
	srv := &Server{
		ctx:          ctx,
		logger:       slogtest.Make(t, nil),
		allowedPorts: []string{"443"},
		refreshProviders: func(context.Context) (ProviderReload, error) {
			if failRefresh {
				return ProviderReload{}, refreshErr
			}
			return reload, nil
		},
	}
	srv.providerRouter.Store(emptyProviderRouter)

	require.NoError(t, srv.Reload(ctx))
	before := srv.loadProviderRouter()
	assert.Equal(t, routedProvider{name: "old", providerType: "openai"}, before.providerFromHost("old.example.com"))

	failRefresh = true
	require.ErrorIs(t, srv.Reload(ctx), refreshErr)

	after := srv.loadProviderRouter()
	assert.Same(t, before, after)
	assert.Equal(t, routedProvider{name: "old", providerType: "openai"}, after.providerFromHost("old.example.com"))
	assert.Equal(t, []string{"old.example.com:443"}, after.mitmHosts)
}

// TestBuildProviderRouter covers the host-and-routing derivation from
// the classified provider reload.
func TestBuildProviderRouter(t *testing.T) {
	t.Parallel()

	t.Run("IncludesEnabledOnly", func(t *testing.T) {
		t.Parallel()

		reload := ProviderReload{Providers: []ReloadedProvider{
			enabledProvider("openai", "api.openai.com"),
			{ProviderOutcome: aibridged.ProviderOutcome{Name: "anthropic", Type: "anthropic", Status: aibridged.ProviderStatusEnabled}, Host: "api.anthropic.com"},
			enabledProvider("custom", "custom-llm.example.com"),
			// Host is populated on the non-enabled rows so the Status
			// guard, not the empty-host guard, is what excludes them.
			{ProviderOutcome: aibridged.ProviderOutcome{Name: "off", Type: "openai", Status: aibridged.ProviderStatusDisabled}, Host: "disabled.example.com"},
			{ProviderOutcome: aibridged.ProviderOutcome{Name: "bad", Type: "openai", Status: aibridged.ProviderStatusError, Err: xerrors.New("nope")}, Host: "errored.example.com"},
		}}

		router, err := buildProviderRouter(reload, []string{"443"})
		require.NoError(t, err)

		assert.Equal(t, routedProvider{name: "openai", providerType: "openai"}, router.providerFromHost("api.openai.com"))
		assert.Equal(t, routedProvider{name: "anthropic", providerType: "anthropic"}, router.providerFromHost("api.anthropic.com"))
		assert.Equal(t, routedProvider{name: "custom", providerType: "openai"}, router.providerFromHost("custom-llm.example.com"))
		assert.Empty(t, router.providerFromHost("unknown.com"))
		assert.Empty(t, router.providerFromHost("disabled.example.com"),
			"disabled provider must not be routable even with a populated Host")
		assert.Empty(t, router.providerFromHost("errored.example.com"),
			"errored provider must not be routable even with a populated Host")

		assert.Contains(t, router.mitmHosts, "api.openai.com:443")
		assert.Contains(t, router.mitmHosts, "api.anthropic.com:443")
		assert.Len(t, router.mitmHosts, 3)
	})

	t.Run("CaseInsensitive", func(t *testing.T) {
		t.Parallel()

		reload := ProviderReload{Providers: []ReloadedProvider{
			{ProviderOutcome: aibridged.ProviderOutcome{Name: "provider", Type: "openai", Status: aibridged.ProviderStatusEnabled}, Host: "API.Example.COM"},
		}}

		router, err := buildProviderRouter(reload, []string{"443"})
		require.NoError(t, err)

		assert.Equal(t, routedProvider{name: "provider", providerType: "openai"}, router.providerFromHost("API.Example.COM"))
		assert.Equal(t, routedProvider{name: "provider", providerType: "openai"}, router.providerFromHost("api.example.com"))
	})

	t.Run("DefensiveDeduplicatesSameHost", func(t *testing.T) {
		t.Parallel()

		// buildProviderRouter is defensive: it tolerates an enabled
		// duplicate by giving the first entry the host (first wins).
		reload := ProviderReload{Providers: []ReloadedProvider{
			enabledProvider("first", "api.example.com"),
			enabledProvider("second", "api.example.com"),
		}}

		router, err := buildProviderRouter(reload, []string{"443"})
		require.NoError(t, err)

		assert.Equal(t, routedProvider{name: "first", providerType: "openai"}, router.providerFromHost("api.example.com"))
	})

	t.Run("SkipsRowsWithEmptyHost", func(t *testing.T) {
		t.Parallel()

		reload := ProviderReload{Providers: []ReloadedProvider{
			{ProviderOutcome: aibridged.ProviderOutcome{Name: "no-host", Type: "openai", Status: aibridged.ProviderStatusEnabled}},
			enabledProvider("good", "api.good.example.com"),
		}}

		router, err := buildProviderRouter(reload, []string{"443"})
		require.NoError(t, err)

		assert.Equal(t, routedProvider{name: "good", providerType: "openai"}, router.providerFromHost("api.good.example.com"))
		assert.Equal(t, []string{"api.good.example.com:443"}, router.mitmHosts)
	})
}
