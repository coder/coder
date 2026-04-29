package xreplicasync

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/nats"
	muxtestutil "github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// fakeReplicaSource is a minimal stand-in for replicasync.Manager. Unlike
// the real Manager which invokes the callback asynchronously, Trigger
// here calls the registered callback synchronously so tests can sequence
// events deterministically without sleeping.
type fakeReplicaSource struct {
	mu       sync.Mutex
	id       uuid.UUID
	replicas []database.Replica
	cbs      map[uint64]func()
	nextID   uint64
}

func newFakeSource(id uuid.UUID, replicas []database.Replica) *fakeReplicaSource {
	return &fakeReplicaSource{id: id, replicas: replicas, cbs: map[uint64]func(){}}
}

func (f *fakeReplicaSource) ID() uuid.UUID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.id
}

func (f *fakeReplicaSource) Regional() []database.Replica {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]database.Replica, len(f.replicas))
	copy(out, f.replicas)
	return out
}

// AddCallback matches the production ReplicaSource contract: it stores
// the callback under a unique ID and returns an idempotent remove
// function. Unlike the real Manager, it does NOT auto-fire on add so
// tests can drive callbacks via Trigger explicitly.
func (f *fakeReplicaSource) AddCallback(cb func()) func() {
	f.mu.Lock()
	if f.cbs == nil {
		f.cbs = map[uint64]func(){}
	}
	id := f.nextID
	f.nextID++
	f.cbs[id] = cb
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		delete(f.cbs, id)
	}
}

// CallbackCount reports how many callbacks are currently registered.
// Tests use this to assert that Close detaches the provider's callback.
func (f *fakeReplicaSource) CallbackCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.cbs)
}

func (f *fakeReplicaSource) SetReplicas(replicas []database.Replica) {
	f.mu.Lock()
	f.replicas = replicas
	f.mu.Unlock()
}

func (f *fakeReplicaSource) Trigger() {
	f.mu.Lock()
	cbs := make([]func(), 0, len(f.cbs))
	for _, cb := range f.cbs {
		cbs = append(cbs, cb)
	}
	f.mu.Unlock()
	for _, cb := range cbs {
		cb()
	}
}

// fakeRefresher is a pubsubRefresher that returns queued errors and
// records every call on a buffered channel for deterministic assertions.
type fakeRefresher struct {
	mu    sync.Mutex
	queue []error
	calls chan struct{}
}

func newFakeRefresher(buf int) *fakeRefresher {
	return &fakeRefresher{calls: make(chan struct{}, buf)}
}

func (f *fakeRefresher) Enqueue(errs ...error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queue = append(f.queue, errs...)
}

func (f *fakeRefresher) RefreshPeers(_ context.Context) error {
	f.mu.Lock()
	var err error
	if len(f.queue) > 0 {
		err = f.queue[0]
		f.queue = f.queue[1:]
	}
	f.mu.Unlock()
	select {
	case f.calls <- struct{}{}:
	default:
	}
	return err
}

func mustWaitCall(ctx context.Context, t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for refresh call: %v", ctx.Err())
	}
}

func mustNotWaitCall(t *testing.T, ch <-chan struct{}, d time.Duration) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("unexpected refresh call observed")
	case <-time.After(d):
	}
}

func newTestLogger(t *testing.T) slog.Logger {
	return slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
}

func newTestProvider(t *testing.T, src ReplicaSource, opts ...func(*Options)) (*Provider, *prometheus.CounterVec) {
	t.Helper()
	failures := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_refresh_failures"}, []string{})
	fn, err := RouteURLFromReplicaHostname("nats", 6222)
	require.NoError(t, err)
	o := Options{
		Logger:          newTestLogger(t),
		Source:          src,
		RouteURL:        fn,
		RefreshFailures: failures.WithLabelValues(),
		RetryMinBackoff: time.Second,
		RetryMaxBackoff: 60 * time.Second,
	}
	for _, fn := range opts {
		fn(&o)
	}
	p, err := New(o)
	require.NoError(t, err)
	return p, failures
}

func mkReplica(id uuid.UUID, hostname string, primary bool) database.Replica {
	return database.Replica{ID: id, Hostname: hostname, Primary: primary}
}

func TestPeers_FiltersNonPrimaryAndSelf(t *testing.T) {
	t.Parallel()
	self := uuid.New()
	other := uuid.New()
	notPrimary := uuid.New()
	src := newFakeSource(self, []database.Replica{
		mkReplica(self, "self-host", true),
		mkReplica(other, "peer-host", true),
		mkReplica(notPrimary, "proxy-host", false),
	})
	p, _ := newTestProvider(t, src)
	t.Cleanup(func() { _ = p.Close() })

	peers, err := p.Peers(context.Background())
	require.NoError(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, "peer-host", peers[0].Name)
	require.Equal(t, "nats://peer-host:6222", peers[0].RouteURL)
}

func TestPeers_RouteURLErrorWrapsReplicaID(t *testing.T) {
	t.Parallel()
	self := uuid.New()
	bad := uuid.New()
	src := newFakeSource(self, []database.Replica{
		mkReplica(bad, "", true), // empty hostname triggers route URL error.
	})
	p, _ := newTestProvider(t, src)
	t.Cleanup(func() { _ = p.Close() })
	_, err := p.Peers(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), bad.String())
}

func TestPeers_FallbackPeerNameFromID(t *testing.T) {
	t.Parallel()
	self := uuid.New()
	other := uuid.New()
	src := newFakeSource(self, []database.Replica{
		mkReplica(other, "advertised-host", true),
	})
	// Use a RouteURLFunc that does not require Hostname so we can
	// supply an empty Hostname while still producing a route URL.
	fn := RouteURLFunc(func(r database.Replica) (string, error) {
		return "nats://10.0.0.1:6222", nil
	})
	p, _ := newTestProvider(t, src, func(o *Options) { o.RouteURL = fn })
	t.Cleanup(func() { _ = p.Close() })

	// Replace replica with empty hostname.
	src.replicas = []database.Replica{{ID: other, Primary: true}}
	peers, err := p.Peers(context.Background())
	require.NoError(t, err)
	require.Len(t, peers, 1)
	require.Equal(t, other.String(), peers[0].Name)
}

func TestPeerRouteFingerprint_StableAcrossOrderAndName(t *testing.T) {
	t.Parallel()
	a := nats.Peer{Name: "alpha", RouteURL: "nats://a:6222"}
	b := nats.Peer{Name: "beta", RouteURL: "nats://b:6222"}
	fp1 := peerRouteFingerprint([]nats.Peer{a, b})
	fp2 := peerRouteFingerprint([]nats.Peer{b, a})
	require.Equal(t, fp1, fp2, "fingerprint should be order independent")

	aRenamed := nats.Peer{Name: "alpha-renamed", RouteURL: "nats://a:6222"}
	fp3 := peerRouteFingerprint([]nats.Peer{aRenamed, b})
	require.Equal(t, fp1, fp3, "name changes must not affect fingerprint")

	c := nats.Peer{Name: "alpha", RouteURL: "nats://c:6222"}
	fp4 := peerRouteFingerprint([]nats.Peer{c, b})
	require.NotEqual(t, fp1, fp4, "route url changes must change fingerprint")
}

func TestProvider_DuplicateTriggerSkipsSecondRefresh(t *testing.T) {
	t.Parallel()
	ctx := muxtestutil.Context(t, muxtestutil.WaitShort)
	self := uuid.New()
	other := uuid.New()
	src := newFakeSource(self, []database.Replica{mkReplica(other, "host-a", true)})
	clk := quartz.NewMock(t)
	p, _ := newTestProvider(t, src, func(o *Options) { o.Clock = clk })
	r := newFakeRefresher(8)
	require.NoError(t, p.startWithRefresher(ctx, r))
	t.Cleanup(func() { _ = p.Close() })

	src.Trigger()
	mustWaitCall(ctx, t, r.calls)

	// Trigger again with no change in the replica set; the worker
	// should observe an unchanged fingerprint and skip the refresh.
	src.Trigger()
	mustNotWaitCall(t, r.calls, 50*time.Millisecond)
}

func TestProvider_MaterialChangeTriggersSecondRefresh(t *testing.T) {
	t.Parallel()
	ctx := muxtestutil.Context(t, muxtestutil.WaitShort)
	self := uuid.New()
	a := uuid.New()
	b := uuid.New()
	src := newFakeSource(self, []database.Replica{mkReplica(a, "host-a", true)})
	p, _ := newTestProvider(t, src)
	r := newFakeRefresher(8)
	require.NoError(t, p.startWithRefresher(ctx, r))
	t.Cleanup(func() { _ = p.Close() })

	src.Trigger()
	mustWaitCall(ctx, t, r.calls)

	src.SetReplicas([]database.Replica{mkReplica(b, "host-b", true)})
	src.Trigger()
	mustWaitCall(ctx, t, r.calls)
}

func TestProvider_RetryBackoffThenSuccessThenReset(t *testing.T) {
	t.Parallel()
	ctx := muxtestutil.Context(t, muxtestutil.WaitShort)
	self := uuid.New()
	a := uuid.New()
	b := uuid.New()
	src := newFakeSource(self, []database.Replica{mkReplica(a, "host-a", true)})
	clk := quartz.NewMock(t)
	trap := clk.Trap().NewTimer("xreplicasync", "refresh")
	defer trap.Close()
	p, failures := newTestProvider(t, src, func(o *Options) { o.Clock = clk })
	r := newFakeRefresher(16)
	r.Enqueue(xerrors.New("transient-1"), xerrors.New("transient-2"), nil)
	require.NoError(t, p.startWithRefresher(ctx, r))
	t.Cleanup(func() { _ = p.Close() })

	src.Trigger()
	mustWaitCall(ctx, t, r.calls)

	call := trap.MustWait(ctx)
	require.Equal(t, time.Second, call.Duration)
	call.MustRelease(ctx)
	clk.Advance(time.Second).MustWait(ctx)
	mustWaitCall(ctx, t, r.calls)

	call = trap.MustWait(ctx)
	require.Equal(t, 2*time.Second, call.Duration)
	call.MustRelease(ctx)
	clk.Advance(2 * time.Second).MustWait(ctx)
	mustWaitCall(ctx, t, r.calls)

	require.Equal(t, 2.0, testutil.ToFloat64(failures.WithLabelValues()))

	// Material change with one transient error then success: backoff
	// must reset to retryMin for the new signal.
	src.SetReplicas([]database.Replica{mkReplica(b, "host-b", true)})
	r.Enqueue(xerrors.New("transient-3"), nil)
	src.Trigger()
	mustWaitCall(ctx, t, r.calls)
	call = trap.MustWait(ctx)
	require.Equal(t, time.Second, call.Duration)
	call.MustRelease(ctx)
	clk.Advance(time.Second).MustWait(ctx)
	mustWaitCall(ctx, t, r.calls)

	require.Equal(t, 3.0, testutil.ToFloat64(failures.WithLabelValues()))
}

func TestProvider_NoEmbeddedServerIsTerminalForOneSignal(t *testing.T) {
	t.Parallel()
	ctx := muxtestutil.Context(t, muxtestutil.WaitShort)
	self := uuid.New()
	a := uuid.New()
	b := uuid.New()
	src := newFakeSource(self, []database.Replica{mkReplica(a, "host-a", true)})
	clk := quartz.NewMock(t)
	trap := clk.Trap().NewTimer("xreplicasync", "refresh")
	defer trap.Close()
	p, failures := newTestProvider(t, src, func(o *Options) { o.Clock = clk })
	r := newFakeRefresher(8)
	r.Enqueue(nats.ErrNoEmbeddedServer, nil)
	require.NoError(t, p.startWithRefresher(ctx, r))
	t.Cleanup(func() { _ = p.Close() })

	src.Trigger()
	mustWaitCall(ctx, t, r.calls)
	// No retry timer should be created and counter should remain zero.
	mustNotWaitCall(t, r.calls, 50*time.Millisecond)
	require.Equal(t, 0.0, testutil.ToFloat64(failures.WithLabelValues()))

	// A new material change should still be processed.
	src.SetReplicas([]database.Replica{mkReplica(b, "host-b", true)})
	src.Trigger()
	mustWaitCall(ctx, t, r.calls)
}

func TestProvider_CloseIdempotent(t *testing.T) {
	t.Parallel()
	ctx := muxtestutil.Context(t, muxtestutil.WaitShort)
	src := newFakeSource(uuid.New(), nil)

	// Close before start.
	p1, _ := newTestProvider(t, src)
	require.NoError(t, p1.Close())
	require.NoError(t, p1.Close())

	// Start then close, then double close.
	p2, _ := newTestProvider(t, src)
	r := newFakeRefresher(2)
	require.NoError(t, p2.startWithRefresher(ctx, r))
	require.NoError(t, p2.Close())
	require.NoError(t, p2.Close())
}

func TestProvider_CloseDetachesCallback(t *testing.T) {
	t.Parallel()
	ctx := muxtestutil.Context(t, muxtestutil.WaitShort)
	self := uuid.New()
	other := uuid.New()
	src := newFakeSource(self, []database.Replica{mkReplica(other, "host-a", true)})
	p, _ := newTestProvider(t, src)
	r := newFakeRefresher(8)
	require.NoError(t, p.startWithRefresher(ctx, r))

	require.Equal(t, 1, src.CallbackCount(), "provider should register exactly one callback")

	src.Trigger()
	mustWaitCall(ctx, t, r.calls)

	require.NoError(t, p.Close())
	require.Equal(t, 0, src.CallbackCount(), "Close must detach the provider callback")

	// Subsequent triggers must not produce refresh calls because the
	// callback is detached and the worker has exited.
	src.Trigger()
	mustNotWaitCall(t, r.calls, 50*time.Millisecond)
}

func TestProvider_StartRequiresPubsub(t *testing.T) {
	t.Parallel()
	src := newFakeSource(uuid.New(), nil)
	p, _ := newTestProvider(t, src)
	t.Cleanup(func() { _ = p.Close() })
	err := p.Start(context.Background(), nil)
	require.Error(t, err)
}
