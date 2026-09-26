package aibridged

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/x/proxy"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestServerKeyPoolStateCollector(t *testing.T) {
	t.Parallel()

	newProvider := func(t *testing.T, name string) aibridge.Provider {
		t.Helper()

		pool, err := keypool.New(name, []string{"key"}, quartz.NewReal(), nil)
		require.NoError(t, err)
		return aibridge.NewOpenAIProvider(config.OpenAI{Name: name, BaseURL: "http://upstream.test", KeyPool: pool})
	}
	gather := func(t *testing.T, registry *prometheus.Registry) []*dto.MetricFamily {
		t.Helper()

		metrics, err := registry.Gather()
		require.NoError(t, err)
		return metrics
	}

	var server Server
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(server.KeyPoolStateCollector()))

	// One registered key-state collector must safely handle startup before mode
	// selection and proxy mode before the first provider snapshot arrives.
	require.Empty(t, gather(t, registry))
	server.backend.Store(&backend{})
	require.Empty(t, gather(t, registry))

	// Interception reloads update metrics from the same registered collector.
	firstProvider := newProvider(t, "first")
	pool, err := NewCachedBridgePool(DefaultPoolOptions, []aibridge.Provider{firstProvider}, slogtest.Make(t, nil), nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })
	server.backend.Store(&backend{pool: pool, keyPools: pool.KeyPools})

	metrics := gather(t, registry)
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 1, "key_pool_state", "first", "valid"))

	secondProvider := newProvider(t, "second")
	pool.ReplaceProviders([]aibridge.Provider{secondProvider})

	metrics = gather(t, registry)
	require.False(t, testutil.PromGaugeGathered(t, metrics, "key_pool_state", "first", "valid"))
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 1, "key_pool_state", "second", "valid"))

	// Replacing proxy snapshots similarly changes the output without
	// registering another collector.
	thirdProvider := newProvider(t, "third")
	firstRouter, err := proxy.NewRouter([]aibridge.Provider{thirdProvider}, slogtest.Make(t, nil))
	require.NoError(t, err)
	server.backend.Store(&backend{proxyRouter: firstRouter, keyPools: firstRouter.KeyPools})

	metrics = gather(t, registry)
	require.False(t, testutil.PromGaugeGathered(t, metrics, "key_pool_state", "second", "valid"))
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 1, "key_pool_state", "third", "valid"))

	fourthProvider := newProvider(t, "fourth")
	secondRouter, err := proxy.NewRouter([]aibridge.Provider{fourthProvider}, slogtest.Make(t, nil))
	require.NoError(t, err)
	server.backend.Store(&backend{proxyRouter: secondRouter, keyPools: secondRouter.KeyPools})

	metrics = gather(t, registry)
	require.False(t, testutil.PromGaugeGathered(t, metrics, "key_pool_state", "third", "valid"))
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 1, "key_pool_state", "fourth", "valid"))
}

// InterceptionPoolForTest returns the active interception pool, if selected.
func (s *Server) InterceptionPoolForTest() Pooler {
	current := s.backend.Load()
	if current == nil {
		return nil
	}
	return current.pool
}

// shutdownPool isolates pool shutdown from request handling.
type shutdownPool struct {
	Pooler
	shutdown func(context.Context) error
}

func (p *shutdownPool) Shutdown(ctx context.Context) error { return p.shutdown(ctx) }

func TestServerShutdownMode(t *testing.T) {
	t.Parallel()
	poolErr := xerrors.New("pool shutdown failed")
	for _, tc := range []struct {
		name         string
		selected     bool
		interception bool
		poolErr      error
	}{
		{name: "Proxy", selected: true},
		{name: "Interception", selected: true, interception: true},
		{name: "InterceptionFailure", selected: true, interception: true, poolErr: poolErr},
		{name: "BeforeSelection"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitShort)
			lifecycleCtx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			calls := 0
			pool := &shutdownPool{shutdown: func(got context.Context) error {
				require.Equal(t, ctx, got)
				calls++
				return tc.poolErr
			}}
			server := &Server{
				lifecycleCtx: lifecycleCtx, cancelFn: cancel,
				logger:   slogtest.Make(t, &slogtest.Options{IgnoreErrors: tc.poolErr != nil}),
				inflight: aibridge.NewInflightGate(slogtest.Make(t, nil)),
			}
			if tc.selected {
				current := &backend{}
				if tc.interception {
					current.pool = pool
				}
				server.backend.Store(current)
			}
			proxy := tc.selected && !tc.interception
			if !proxy {
				// An unused proxy gate must not delay interception or startup cleanup.
				release, ok := server.inflight.Admit()
				require.True(t, ok)
				defer release()
			}
			require.ErrorIs(t, server.Shutdown(ctx), tc.poolErr)
			expectedCalls := 0
			if tc.interception {
				expectedCalls = 1
			}
			require.Equal(t, expectedCalls, calls)
			release, ok := server.inflight.Admit()
			require.Equal(t, !proxy, ok, "only proxy mode shuts down the proxy gate")
			if ok {
				release()
			}
			_ = server.Shutdown(ctx)
			require.Equal(t, expectedCalls, calls, "owned pool is shut down only once")
		})
	}
}
