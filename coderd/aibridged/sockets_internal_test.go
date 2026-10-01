package aibridged

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func newTestSocketRegistry(t *testing.T, limits socketLimits) (*SocketRegistry, *quartz.Mock, *aibridge.Metrics) {
	t.Helper()
	clock := quartz.NewMock(t)
	metrics := aibridge.NewMetrics(prometheus.NewRegistry())
	reg := newSocketRegistry(clock, metrics, limits)
	t.Cleanup(func() { _ = reg.Shutdown(context.Background()) })
	return reg, clock, metrics
}

// TestSocketRegistryCaps requires that a user cannot hold more than its
// share of sockets and a replica no more than its total, that every refusal
// is counted, and that a released lease frees its place exactly once.
func TestSocketRegistryCaps(t *testing.T) {
	t.Parallel()
	reg, _, metrics := newTestSocketRegistry(t, socketLimits{perActor: 2, perReplica: 3, lifetime: maxSocketLifetime})
	ctx := testutil.Context(t, testutil.WaitShort)
	open := func() float64 { return promtest.ToFloat64(metrics.ResponsesWebSocketsOpen.WithLabelValues("openai")) }
	refused := func(reason string) float64 {
		return promtest.ToFloat64(metrics.ResponsesWebSocketRefusals.WithLabelValues("openai", reason))
	}

	a1, err := reg.Acquire(ctx, "a", "openai")
	require.NoError(t, err)
	a2, err := reg.Acquire(ctx, "a", "openai")
	require.NoError(t, err)
	_, err = reg.Acquire(ctx, "a", "openai")
	require.ErrorIs(t, err, ErrSocketActorLimit)
	require.Equal(t, float64(1), refused("actor_limit"))

	// A lease counts from Acquire, before its socket connected.
	b1, err := reg.Acquire(ctx, "b", "openai")
	require.NoError(t, err)
	_, err = reg.Acquire(ctx, "c", "openai")
	require.ErrorIs(t, err, ErrSocketReplicaLimit)
	require.Equal(t, float64(1), refused("replica_limit"))
	require.Equal(t, float64(3), open())

	// Releasing twice frees one place.
	a1.Release()
	a1.Release()
	require.ErrorIs(t, a1.Context().Err(), context.Canceled)
	require.Equal(t, float64(2), open())
	c1, err := reg.Acquire(ctx, "c", "openai")
	require.NoError(t, err)
	_, err = reg.Acquire(ctx, "a", "openai")
	require.ErrorIs(t, err, ErrSocketReplicaLimit)

	for _, lease := range []*SocketLease{a2, b1, c1} {
		lease.Release()
	}
	require.Equal(t, float64(0), open())
	require.Empty(t, reg.leases)
	require.Empty(t, reg.perActor)
}

// TestSocketLeaseLifetime requires that a lease context ends with
// ErrSocketLifetime at the maximum lifetime, not before, and that the
// expired lease still holds its place until released.
func TestSocketLeaseLifetime(t *testing.T) {
	t.Parallel()
	reg, clock, _ := newTestSocketRegistry(t, socketLimits{perActor: 1, perReplica: 1, lifetime: maxSocketLifetime})
	ctx := testutil.Context(t, testutil.WaitShort)
	lease, err := reg.Acquire(ctx, "a", "openai")
	require.NoError(t, err)

	clock.Advance(maxSocketLifetime - 1).MustWait(ctx)
	require.NoError(t, lease.Context().Err())
	clock.Advance(1).MustWait(ctx)
	require.ErrorIs(t, context.Cause(lease.Context()), ErrSocketLifetime)

	_, err = reg.Acquire(ctx, "a", "openai")
	require.ErrorIs(t, err, ErrSocketActorLimit)
	lease.Release()
	again, err := reg.Acquire(ctx, "a", "openai")
	require.NoError(t, err)
	again.Release()
}

// TestSocketRegistryShutdown requires that shutdown ends every lease with
// ErrShutdown, refuses new leases, and waits for every lease to be released
// or its deadline.
func TestSocketRegistryShutdown(t *testing.T) {
	t.Parallel()
	reg, _, _ := newTestSocketRegistry(t, defaultSocketLimits)
	ctx := testutil.Context(t, testutil.WaitShort)
	first, err := reg.Acquire(ctx, "a", "openai")
	require.NoError(t, err)
	second, err := reg.Acquire(ctx, "b", "openai")
	require.NoError(t, err)

	expired, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, reg.Shutdown(expired), context.Canceled)
	for _, lease := range []*SocketLease{first, second} {
		require.ErrorIs(t, context.Cause(lease.Context()), ErrShutdown)
	}
	_, err = reg.Acquire(ctx, "c", "openai")
	require.ErrorIs(t, err, ErrShutdown)

	// Shutdown still waits while any lease is held.
	first.Release()
	require.ErrorIs(t, reg.Shutdown(expired), context.Canceled)
	second.Release()
	require.NoError(t, reg.Shutdown(ctx))
}

// TestSocketRegistryAcquireRacesShutdown requires that every lease granted
// while shutdown runs is ended by it, so shutdown never leaks one.
func TestSocketRegistryAcquireRacesShutdown(t *testing.T) {
	t.Parallel()
	reg, _, _ := newTestSocketRegistry(t, defaultSocketLimits)
	ctx := testutil.Context(t, testutil.WaitShort)

	const acquirers = 64
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, acquirers)
	for i := range acquirers {
		wg.Go(func() {
			<-start
			lease, err := reg.Acquire(ctx, string(rune('a'+i%26)), "openai")
			if err != nil {
				errs <- err
				return
			}
			// A socket ends when its lease context does.
			<-lease.Context().Done()
			errs <- context.Cause(lease.Context())
			lease.Release()
		})
	}
	close(start)
	require.NoError(t, reg.Shutdown(ctx))
	wg.Wait()
	close(errs)
	for err := range errs {
		require.ErrorIs(t, err, ErrShutdown)
	}
	require.Empty(t, reg.leases)
	require.Empty(t, reg.perActor)
}

// TestSocketLeaseOutlivesBridge requires that a lease acquired while serving
// an upgrade through a RequestBridge keeps its context, and the request's
// actor, when the bridge is evicted. Eviction runs RequestBridge.Shutdown,
// which waits for the bridge's in-flight gate and then closes it, canceling
// every request context the gate handed out.
func TestSocketLeaseOutlivesBridge(t *testing.T) {
	t.Parallel()
	reg, _, _ := newTestSocketRegistry(t, defaultSocketLimits)
	ctx := testutil.Context(t, testutil.WaitShort)
	gate := aibridge.NewInflightGate(slogtest.Make(t, nil))

	leases := make(chan *SocketLease, 1)
	served := make(chan struct{})
	handler := gate.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		lease, err := reg.Acquire(r.Context(), "a", "openai")
		if !assertNoError(t, err) {
			close(leases)
			return
		}
		leases <- lease
		<-r.Context().Done()
	}))
	go func() {
		defer close(served)
		req := httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)
		req = req.WithContext(aibcontext.AsActor(ctx, "a", "", nil))
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}()
	lease := testutil.TryReceive(ctx, t, leases)
	require.NotNil(t, lease)
	defer lease.Release()

	// RequestBridge.Shutdown with its deadline passed.
	expired, cancel := context.WithCancel(ctx)
	cancel()
	require.Error(t, gate.Shutdown(expired))
	gate.Close()
	_ = testutil.TryReceive(ctx, t, served)

	require.NoError(t, lease.Context().Err())
	require.Equal(t, "a", aibcontext.ActorIDFromContext(lease.Context()))
}

// TestServerShutdownEndsSockets requires that the server's shutdown ends its
// sockets, since they outlive the bridge pool, and waits for them.
func TestServerShutdownEndsSockets(t *testing.T) {
	t.Parallel()
	srv := newProxyTestServer(t, http.NotFoundHandler())
	ctx := testutil.Context(t, testutil.WaitShort)
	lease, err := srv.Sockets().Acquire(ctx, "a", "openai")
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- srv.Shutdown(ctx) }()
	<-lease.Context().Done()
	require.ErrorIs(t, context.Cause(lease.Context()), ErrShutdown)
	// The connection to coderd stays up until the socket released its
	// lease, so a closing socket can still record.
	select {
	case <-srv.Done():
		t.Fatal("server lifecycle ended before the socket was released")
	default:
	}
	lease.Release()
	require.NoError(t, testutil.TryReceive(ctx, t, done))
	_ = testutil.TryReceive(ctx, t, srv.Done())
	_, err = srv.Sockets().Acquire(ctx, "a", "openai")
	require.ErrorIs(t, err, ErrShutdown)
}

// assertNoError reports err on t from a goroutine other than the test's.
func assertNoError(t *testing.T, err error) bool {
	t.Helper()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return false
	}
	return true
}
