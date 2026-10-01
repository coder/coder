package aibridged

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

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
			require.ErrorIs(t, server.Shutdown(ctx), tc.poolErr)
			expectedCalls := 0
			if tc.interception {
				expectedCalls = 1
			}
			require.Equal(t, expectedCalls, calls)
			release, ok := server.inflight.Admit()
			require.False(t, ok, "shutdown closes the gate even when it was unused")
			if ok {
				release()
			}
			_ = server.Shutdown(ctx)
			require.Equal(t, expectedCalls, calls, "owned pool is shut down only once")
		})
	}
}

// TestServerShutdownDoesNotWaitForStalledConnectLoop asserts Shutdown bounds
// its wait for the connect loop by the caller's context.
func TestServerShutdownDoesNotWaitForStalledConnectLoop(t *testing.T) {
	t.Parallel()

	lifecycleCtx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	dialStarted := make(chan struct{})
	releaseDial := make(chan struct{})
	server := &Server{
		lifecycleCtx: lifecycleCtx,
		cancelFn:     cancel,
		logger:       slogtest.Make(t, nil),
		inflight:     aibridge.NewInflightGate(slogtest.Make(t, nil)),
		// The dialer ignores cancellation to model a stalled connection attempt.
		clientDialer: func(context.Context) (DRPCClient, error) {
			close(dialStarted)
			<-releaseDial
			return nil, xerrors.New("dial released")
		},
	}
	connectDone := make(chan struct{})
	server.wg.Add(1)
	go func() {
		defer close(connectDone)
		server.connect()
	}()
	t.Cleanup(func() {
		close(releaseDial)
		select {
		case <-connectDone:
		case <-time.After(testutil.WaitShort):
			t.Error("connect loop did not exit after the dial was released")
		}
	})
	testCtx := testutil.Context(t, testutil.WaitShort)
	testutil.TryReceive(testCtx, t, dialStarted)

	shutdownCtx, cancelShutdown := context.WithCancel(t.Context())
	cancelShutdown()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- server.Shutdown(shutdownCtx) }()
	require.ErrorIs(t, testutil.RequireReceive(testCtx, t, shutdownDone), context.Canceled)
	require.ErrorIs(t, context.Cause(lifecycleCtx), ErrShutdown)
	select {
	case <-connectDone:
		t.Fatal("connect loop exited before the stalled dial was released")
	default:
	}
}

type blockingNameProvider struct {
	aibridge.Provider
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingNameProvider) Name() string {
	p.once.Do(func() { close(p.started) })
	<-p.release
	return p.Provider.Name()
}

func TestReplaceProvidersDuringShutdownKeepsAdmissionClosed(t *testing.T) {
	t.Parallel()

	server := newProxyTestServer(t, http.NotFoundHandler())
	require.NoError(t, server.ReplaceProviders(t.Context(), nil))
	retained, err := server.GetRequestHandler(t.Context(), Request{})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	retained.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/candidate/v1/models", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)

	provider := &blockingNameProvider{
		Provider: aibridge.NewOpenAIProvider(config.OpenAI{
			Name:    "candidate",
			BaseURL: "http://upstream.test",
		}),
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	unblock := sync.OnceFunc(func() { close(provider.release) })
	var replaceErr error
	replaceDone := make(chan struct{})
	go func() {
		defer close(replaceDone)
		replaceErr = server.ReplaceProviders(t.Context(), []aibridge.Provider{provider})
	}()
	t.Cleanup(func() {
		unblock()
		select {
		case <-replaceDone:
		case <-time.After(testutil.WaitShort):
			t.Error("provider reload did not finish after release")
		}
	})
	testCtx := testutil.Context(t, testutil.WaitShort)
	testutil.TryReceive(testCtx, t, provider.started)

	require.NoError(t, server.Shutdown(testCtx))
	unblock()
	testutil.TryReceive(testCtx, t, replaceDone)
	if replaceErr != nil {
		require.ErrorIs(t, replaceErr, ErrShutdown)
	}
	acquired, err := server.GetRequestHandler(testCtx, Request{})
	require.NoError(t, err)

	// Whether reload publishes or rejects the snapshot, all paths stay closed.
	for _, tc := range []struct {
		name    string
		handler http.Handler
	}{
		{name: "Retained", handler: retained},
		{name: "CurrentSnapshot", handler: server.backend.Load().proxyRouter},
		{name: "AcquiredAfterShutdown", handler: acquired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.NotNil(t, tc.handler)
			rec := httptest.NewRecorder()
			tc.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/candidate/v1/models", nil))
			require.Equal(t, http.StatusServiceUnavailable, rec.Code)
			require.Equal(t, "AI Gateway is shutting down\n", rec.Body.String())
		})
	}
}
