package aibridged

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
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
	firstRouter, err := proxy.NewRouter([]aibridge.Provider{thirdProvider}, recorder.NewLogRecorder(slogtest.Make(t, nil), nil), slogtest.Make(t, nil), nil, nil)
	require.NoError(t, err)
	server.backend.Store(&backend{proxyRouter: firstRouter, keyPools: firstRouter.KeyPools})

	metrics = gather(t, registry)
	require.False(t, testutil.PromGaugeGathered(t, metrics, "key_pool_state", "second", "valid"))
	require.True(t, testutil.PromGaugeHasValue(t, metrics, 1, "key_pool_state", "third", "valid"))

	fourthProvider := newProvider(t, "fourth")
	secondRouter, err := proxy.NewRouter([]aibridge.Provider{fourthProvider}, recorder.NewLogRecorder(slogtest.Make(t, nil), nil), slogtest.Make(t, nil), nil, nil)
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
			require.False(t, ok, "shutdown closes request admission in every mode")
			require.Nil(t, release)
			_ = server.Shutdown(ctx)
			require.Equal(t, expectedCalls, calls, "owned pool is shut down only once")
		})
	}
}

func TestServerShutdownDrainsBeforeCancelingLifecycle(t *testing.T) {
	t.Parallel()

	for _, interception := range []bool{false, true} {
		name := "Proxy"
		if interception {
			name = "Interception"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			lifecycleCtx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			server := &Server{
				lifecycleCtx: lifecycleCtx,
				cancelFn:     cancel,
				logger:       slogtest.Make(t, nil),
				inflight:     aibridge.NewInflightGate(slogtest.Make(t, nil)),
			}

			started := make(chan struct{})
			release := make(chan struct{})
			baseHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				close(started)
				<-release
				w.WriteHeader(http.StatusNoContent)
			})
			if interception {
				server.backend.Store(&backend{pool: &shutdownPool{
					Pooler: &handlerPool{handler: baseHandler},
					shutdown: func(context.Context) error {
						return nil
					},
				}})
			} else {
				server.backend.Store(&backend{proxyRouter: server.inflight.Middleware(baseHandler)})
			}

			handler, err := server.GetRequestHandler(t.Context(), Request{})
			require.NoError(t, err)
			requestDone := make(chan int, 1)
			go func() {
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
				requestDone <- rec.Code
			}()
			testCtx := testutil.Context(t, testutil.WaitShort)
			testutil.TryReceive(testCtx, t, started)

			shutdownDone := make(chan error, 1)
			go func() {
				shutdownDone <- server.Shutdown(testCtx)
			}()
			require.Eventually(t, func() bool {
				probeRelease, ok := server.inflight.Admit()
				if ok {
					probeRelease()
				}
				return !ok
			}, testutil.WaitShort, testutil.IntervalFast)
			require.NoError(t, lifecycleCtx.Err(), "DRPC lifecycle must remain available while draining")
			select {
			case err := <-shutdownDone:
				t.Fatalf("shutdown returned before the admitted request completed: %v", err)
			default:
			}

			refused := httptest.NewRecorder()
			handler.ServeHTTP(refused, httptest.NewRequest(http.MethodPost, "/", nil))
			require.Equal(t, http.StatusServiceUnavailable, refused.Code)
			require.Equal(t, "AI Gateway is shutting down\n", refused.Body.String())

			close(release)
			require.Equal(t, http.StatusNoContent, testutil.TryReceive(testCtx, t, requestDone))
			require.NoError(t, testutil.TryReceive(testCtx, t, shutdownDone))
			require.ErrorIs(t, context.Cause(lifecycleCtx), ErrShutdown)
		})
	}
}

type handlerPool struct {
	Pooler
	handler http.Handler
}

func (p *handlerPool) Acquire(context.Context, Request, ClientFunc, MCPProxyBuilder) (http.Handler, error) {
	return p.handler, nil
}

func (*handlerPool) Shutdown(context.Context) error { return nil }

type countingAcquirePool struct {
	Pooler
	calls atomic.Int32
}

func (p *countingAcquirePool) Acquire(context.Context, Request, ClientFunc, MCPProxyBuilder) (http.Handler, error) {
	p.calls.Add(1)
	return http.NotFoundHandler(), nil
}

type blockingAcquirePool struct {
	Pooler
	started chan struct{}
}

func (p *blockingAcquirePool) Acquire(ctx context.Context, _ Request, _ ClientFunc, _ MCPProxyBuilder) (http.Handler, error) {
	close(p.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (*blockingAcquirePool) Shutdown(context.Context) error { return nil }

func TestGetRequestHandlerRefusesBeforeLegacyAcquire(t *testing.T) {
	t.Parallel()

	pool := &countingAcquirePool{}
	server := &Server{
		lifecycleCtx: context.Background(),
		inflight:     aibridge.NewInflightGate(slogtest.Make(t, nil)),
	}
	server.backend.Store(&backend{pool: pool})
	server.shuttingDown.Store(true)

	handler, err := server.GetRequestHandler(t.Context(), Request{})
	require.NoError(t, err)
	require.Zero(t, pool.calls.Load(), "shutdown must not acquire or build a legacy bridge")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "AI Gateway is shutting down\n", rec.Body.String())
}

func TestGetRequestHandlerFatalLifecycleStillTracksServing(t *testing.T) {
	t.Parallel()

	lifecycleCtx, cancel := context.WithCancelCause(t.Context())
	cancel(xerrors.New("fatal connection error"))
	started := make(chan struct{})
	release := make(chan struct{})
	pool := &handlerPool{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})}
	server := &Server{
		lifecycleCtx: lifecycleCtx,
		cancelFn:     cancel,
		logger:       slogtest.Make(t, nil),
		inflight:     aibridge.NewInflightGate(slogtest.Make(t, nil)),
	}
	server.backend.Store(&backend{pool: pool})

	handler, err := server.GetRequestHandler(t.Context(), Request{})
	require.NoError(t, err)
	requestDone := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
		requestDone <- rec.Code
	}()
	testCtx := testutil.Context(t, testutil.WaitShort)
	testutil.TryReceive(testCtx, t, started)

	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- server.Shutdown(testCtx)
	}()
	select {
	case err := <-shutdownDone:
		t.Fatalf("shutdown returned before fatal-lifecycle request completed: %v", err)
	default:
	}
	close(release)
	require.Equal(t, http.StatusNoContent, testutil.TryReceive(testCtx, t, requestDone))
	require.NoError(t, testutil.TryReceive(testCtx, t, shutdownDone))
}

func TestGetRequestHandlerPreAcquiredHandlerRefusesAfterShutdown(t *testing.T) {
	t.Parallel()

	lifecycleCtx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	var served atomic.Bool
	pool := &handlerPool{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served.Store(true)
		w.WriteHeader(http.StatusNoContent)
	})}
	server := &Server{
		lifecycleCtx: lifecycleCtx,
		cancelFn:     cancel,
		logger:       slogtest.Make(t, nil),
		inflight:     aibridge.NewInflightGate(slogtest.Make(t, nil)),
	}
	server.backend.Store(&backend{pool: pool})

	handler, err := server.GetRequestHandler(t.Context(), Request{})
	require.NoError(t, err)
	require.NoError(t, server.Shutdown(testutil.Context(t, testutil.WaitShort)))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "AI Gateway is shutting down\n", rec.Body.String())
	require.False(t, served.Load(), "a handler acquired but not served before shutdown is not admitted")
}

func TestGetRequestHandlerAcquireIsCanceledAtShutdownDeadline(t *testing.T) {
	t.Parallel()

	lifecycleCtx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	pool := &blockingAcquirePool{started: make(chan struct{})}
	server := &Server{
		lifecycleCtx: lifecycleCtx,
		cancelFn:     cancel,
		logger:       slogtest.Make(t, nil),
		inflight:     aibridge.NewInflightGate(slogtest.Make(t, nil)),
	}
	server.backend.Store(&backend{pool: pool})

	acquireDone := make(chan error, 1)
	go func() {
		_, err := server.GetRequestHandler(t.Context(), Request{})
		acquireDone <- err
	}()
	testCtx := testutil.Context(t, testutil.WaitShort)
	testutil.TryReceive(testCtx, t, pool.started)

	shutdownCtx, shutdownCancel := context.WithCancel(t.Context())
	shutdownCancel()
	require.ErrorIs(t, server.Shutdown(shutdownCtx), context.Canceled)
	require.ErrorIs(t, testutil.TryReceive(testCtx, t, acquireDone), context.Canceled)
}

func TestConnectLoopShutdownExitDoesNotCancelLifecycle(t *testing.T) {
	t.Parallel()

	lifecycleCtx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	server := &Server{
		lifecycleCtx: lifecycleCtx,
		cancelFn:     cancel,
		logger:       slogtest.Make(t, nil),
		clientDialer: func(context.Context) (DRPCClient, error) {
			t.Error("shutdown connect loop must not dial")
			return nil, ErrShutdown
		},
	}
	server.shuttingDown.Store(true)
	server.wg.Add(1)
	server.connect()

	require.NoError(t, lifecycleCtx.Err(), "connect loop must leave lifecycle live during graceful drain")
}

func TestServerShutdownDeadlineDoesNotWaitForBackendMu(t *testing.T) {
	t.Parallel()

	lifecycleCtx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	server := &Server{
		lifecycleCtx: lifecycleCtx,
		cancelFn:     cancel,
		logger:       slogtest.Make(t, nil),
		inflight:     aibridge.NewInflightGate(slogtest.Make(t, nil)),
	}
	var cleanups atomic.Int32
	server.backend.Store(&backend{closeIdleConns: func() { cleanups.Add(1) }})
	server.backendMu.Lock()
	defer server.backendMu.Unlock()

	ctx, cancelShutdown := context.WithCancel(t.Context())
	cancelShutdown()
	require.ErrorIs(t, server.Shutdown(ctx), context.Canceled)
	require.ErrorIs(t, context.Cause(lifecycleCtx), ErrShutdown)
	require.Equal(t, int32(1), cleanups.Load(), "shutdown cleans the snapshot without waiting for backend construction")
}

func TestServerShutdownDeadlineCancelsInflight(t *testing.T) {
	t.Parallel()

	for _, interception := range []bool{false, true} {
		name := "Proxy"
		if interception {
			name = "Interception"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			lifecycleCtx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			server := &Server{
				lifecycleCtx: lifecycleCtx,
				cancelFn:     cancel,
				logger:       slogtest.Make(t, nil),
				inflight:     aibridge.NewInflightGate(slogtest.Make(t, nil)),
			}

			started := make(chan struct{})
			var endAttempts atomic.Int32
			baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-r.Context().Done()
				endAttempts.Add(1)
				http.Error(w, r.Context().Err().Error(), http.StatusServiceUnavailable)
			})
			if interception {
				server.backend.Store(&backend{pool: &shutdownPool{
					Pooler: &handlerPool{handler: baseHandler},
					shutdown: func(context.Context) error {
						return nil
					},
				}})
			} else {
				server.backend.Store(&backend{proxyRouter: server.inflight.Middleware(baseHandler)})
			}

			handler, err := server.GetRequestHandler(t.Context(), Request{})
			require.NoError(t, err)
			requestDone := make(chan int, 1)
			go func() {
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
				requestDone <- rec.Code
			}()
			testCtx := testutil.Context(t, testutil.WaitShort)
			testutil.TryReceive(testCtx, t, started)

			shutdownCtx, shutdownCancel := context.WithCancel(t.Context())
			shutdownCancel()
			require.ErrorIs(t, server.Shutdown(shutdownCtx), context.Canceled)
			require.Equal(t, http.StatusServiceUnavailable, testutil.TryReceive(testCtx, t, requestDone))
			require.Equal(t, int32(1), endAttempts.Load(), "canceled handler attempts finalization exactly once")
			require.ErrorIs(t, context.Cause(lifecycleCtx), ErrShutdown)
		})
	}
}

func TestShutdownSynchronizesWithProviderReplacement(t *testing.T) {
	t.Parallel()

	lifecycleCtx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	server := &Server{
		lifecycleCtx: lifecycleCtx,
		cancelFn:     cancel,
		logger:       slogtest.Make(t, nil),
		inflight:     aibridge.NewInflightGate(slogtest.Make(t, nil)),
	}
	var cleanups atomic.Int32
	original := &backend{closeIdleConns: func() { cleanups.Add(1) }}
	server.backend.Store(original)

	server.backendMu.Lock()
	replaceDone := make(chan error, 1)
	go func() {
		replaceDone <- server.ReplaceProviders(t.Context(), nil)
	}()

	testCtx := testutil.Context(t, testutil.WaitShort)
	require.NoError(t, server.Shutdown(testCtx), "shutdown must not wait for provider construction")
	require.Same(t, original, server.backend.Load())
	require.Equal(t, int32(1), cleanups.Load(), "shutdown cleans the synchronized current snapshot")

	server.backendMu.Unlock()
	require.ErrorIs(t, testutil.TryReceive(testCtx, t, replaceDone), ErrShutdown)
	require.Same(t, original, server.backend.Load(), "no provider snapshot may publish after shutdown begins")
	require.Equal(t, int32(1), cleanups.Load(), "rejected replacement must not clean the current snapshot again")
}
