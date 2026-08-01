package agentrunonce_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentrunonce"
	"github.com/coder/coder/v2/testutil"
)

const testRetention = time.Hour

type reserveResult struct {
	outcome agentrunonce.Outcome[string, string]
	err     error
}

func TestReserveOwnsThenAttaches(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)

	outcome, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)
	require.NotNil(t, outcome.Ticket)

	outcome.Ticket.Publish("value")

	again, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)
	require.Nil(t, again.Ticket)
	require.Equal(t, "value", again.Value)
}

func TestReserveRejectsDifferentInputWhilePending(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)

	outcome, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)
	require.NotNil(t, outcome.Ticket)

	// Answered without waiting out the pending reservation.
	_, err = registry.Reserve(context.Background(), "key", "other")
	require.ErrorIs(t, err, agentrunonce.ErrInputMismatch)

	outcome.Ticket.Publish("value")

	_, err = registry.Reserve(context.Background(), "key", "other")
	require.ErrorIs(t, err, agentrunonce.ErrInputMismatch)
}

func TestReserveWaitsForPublish(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)

	held, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)

	attached := make(chan reserveResult, 1)
	go func() {
		outcome, err := registry.Reserve(context.Background(), "key", "fp")
		attached <- reserveResult{outcome: outcome, err: err}
	}()

	held.Ticket.Publish("value")

	got := testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, attached)
	require.NoError(t, got.err)
	require.Equal(t, "value", got.outcome.Value)
}

func TestReserveTakesOverReleasedReservation(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)

	held, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)

	second := make(chan reserveResult, 1)
	go func() {
		outcome, err := registry.Reserve(context.Background(), "key", "fp")
		second <- reserveResult{outcome: outcome, err: err}
	}()

	// The operation never got underway, so the waiter must take over
	// the key rather than attach to a value that will never exist.
	held.Ticket.Release()

	got := testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, second)
	require.NoError(t, got.err)
	require.NotNil(t, got.outcome.Ticket)
}

func TestReserveWaitHonorsCancellation(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)

	_, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	waiter := make(chan error, 1)
	go func() {
		_, err := registry.Reserve(ctx, "key", "fp")
		waiter <- err
	}()
	cancel()

	err = testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, waiter)
	require.ErrorIs(t, err, agentrunonce.ErrPublicationPending)
	require.ErrorIs(t, err, context.Canceled)
}

func TestCloseUnblocksWaiterAndRefusesNewKeys(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)

	_, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)

	waiter := make(chan error, 1)
	go func() {
		_, err := registry.Reserve(context.Background(), "key", "fp")
		waiter <- err
	}()

	registry.Close()

	err = testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, waiter)
	require.ErrorIs(t, err, agentrunonce.ErrPublicationPending)

	_, err = registry.Reserve(context.Background(), "other", "fp")
	require.ErrorIs(t, err, agentrunonce.ErrClosed)
}

func TestConcurrentReserveIssuesOneTicket(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)

	const callers = 8
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		tickets int
		values  []string
		errs    []error
		publish = make(chan struct{})
	)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcome, err := registry.Reserve(context.Background(), "key", "fp")
			mu.Lock()
			switch {
			case err != nil:
				errs = append(errs, err)
			case outcome.Ticket != nil:
				tickets++
			default:
				values = append(values, outcome.Value)
			}
			mu.Unlock()
			if err == nil && outcome.Ticket != nil {
				<-publish
				outcome.Ticket.Publish("value")
			}
		}()
	}

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return tickets == 1
	}, testutil.WaitShort, testutil.IntervalFast)
	close(publish)
	wg.Wait()

	require.Empty(t, errs)
	require.Equal(t, 1, tickets)
	require.Len(t, values, callers-1)
	for _, value := range values {
		require.Equal(t, "value", value)
	}
}

func TestRetentionEvictsCompletedReservations(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)
	start := time.Now()

	outcome, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)
	outcome.Ticket.Publish("value")

	// A published reservation whose operation is still in flight is
	// retained regardless of age: retention starts at completion.
	require.Empty(t, registry.Reap(start.Add(testRetention*2)))
	require.Equal(t, 1, registry.Len())

	registry.Complete("key", start)
	require.Empty(t, registry.Reap(start.Add(testRetention)))

	require.Equal(t, []string{"value"}, registry.Reap(start.Add(testRetention).Add(time.Second)))
	require.Zero(t, registry.Len())

	fresh, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)
	require.NotNil(t, fresh.Ticket)
}

func TestCompleteIgnoresPendingReservation(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)
	start := time.Now()

	_, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)

	registry.Complete("key", start)
	registry.Complete("missing", start)

	require.Empty(t, registry.Reap(start.Add(testRetention*2)))
	require.Equal(t, 1, registry.Len())
}

func TestForgetDropsPublishedButKeepsPending(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)

	pending, err := registry.Reserve(context.Background(), "pending", "fp")
	require.NoError(t, err)

	published, err := registry.Reserve(context.Background(), "published", "fp")
	require.NoError(t, err)
	published.Ticket.Publish("value")

	// Dropping a pending reservation would let a second caller
	// perform an operation another caller is already performing.
	registry.Forget("pending", pending.Generation)
	registry.Forget("published", published.Generation)

	require.Equal(t, 1, registry.Len())

	outcome, err := registry.Reserve(context.Background(), "published", "fp")
	require.NoError(t, err)
	require.NotNil(t, outcome.Ticket)

	pending.Ticket.Release()
}

// The two adapters below exercise the seam the registry exists for:
// one operation publishes a handle to a live resource it keeps
// managing, another publishes a plain response value. Neither shape
// is known to the registry.

type fakeProcess struct {
	id      string
	running bool
}

type fakeProcessAdapter struct {
	registry *agentrunonce.Registry[string, string]
	mu       sync.Mutex
	procs    map[string]*fakeProcess
	spawns   int
}

func newFakeProcessAdapter() *fakeProcessAdapter {
	return &fakeProcessAdapter{
		registry: agentrunonce.NewRegistry[string, string](testRetention),
		procs:    map[string]*fakeProcess{},
	}
}

func (a *fakeProcessAdapter) start(ctx context.Context, key, command string) (*fakeProcess, bool, error) {
	outcome, err := a.registry.Reserve(ctx, key, command)
	if err != nil {
		return nil, false, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if outcome.Ticket == nil {
		return a.procs[outcome.Value], true, nil
	}
	a.spawns++
	proc := &fakeProcess{id: command + "-proc", running: true}
	a.procs[proc.id] = proc
	outcome.Ticket.Publish(proc.id)
	return proc, false, nil
}

func (a *fakeProcessAdapter) exit(key, id string, now time.Time) {
	a.mu.Lock()
	a.procs[id].running = false
	a.mu.Unlock()
	a.registry.Complete(key, now)
}

func TestAdapterPublishingLiveHandle(t *testing.T) {
	t.Parallel()

	adapter := newFakeProcessAdapter()
	start := time.Now()

	proc, attached, err := adapter.start(context.Background(), "key", "run")
	require.NoError(t, err)
	require.False(t, attached)

	again, attached, err := adapter.start(context.Background(), "key", "run")
	require.NoError(t, err)
	require.True(t, attached)
	require.Same(t, proc, again)
	require.Equal(t, 1, adapter.spawns)

	// The handle stays attachable after the resource stops, so a
	// repeated caller still observes the outcome it missed.
	adapter.exit("key", proc.id, start)
	exited, attached, err := adapter.start(context.Background(), "key", "run")
	require.NoError(t, err)
	require.True(t, attached)
	require.False(t, exited.running)
	require.Equal(t, 1, adapter.spawns)

	for _, id := range adapter.registry.Reap(start.Add(testRetention).Add(time.Second)) {
		delete(adapter.procs, id)
	}
	_, attached, err = adapter.start(context.Background(), "key", "run")
	require.NoError(t, err)
	require.False(t, attached)
	require.Equal(t, 2, adapter.spawns)
}

type fakeEditResult struct {
	paths []string
}

type fakeEditAdapter struct {
	registry *agentrunonce.Registry[string, fakeEditResult]
	mu       sync.Mutex
	applied  int
}

func TestAdapterPublishingPlainValue(t *testing.T) {
	t.Parallel()

	adapter := &fakeEditAdapter{registry: agentrunonce.NewRegistry[string, fakeEditResult](testRetention)}
	start := time.Now()

	edit := func(key, fingerprint string) (fakeEditResult, error) {
		outcome, err := adapter.registry.Reserve(context.Background(), key, fingerprint)
		if err != nil {
			return fakeEditResult{}, err
		}
		if outcome.Ticket == nil {
			return outcome.Value, nil
		}
		adapter.mu.Lock()
		adapter.applied++
		adapter.mu.Unlock()
		result := fakeEditResult{paths: []string{"main.go"}}
		outcome.Ticket.Publish(result)
		// A value operation is terminal as soon as it produces its
		// result, unlike a process that stays live after publishing.
		adapter.registry.Complete(key, start)
		return result, nil
	}

	first, err := edit("key", "fp")
	require.NoError(t, err)
	require.Equal(t, []string{"main.go"}, first.paths)

	// The replayed call returns the recorded result rather than
	// applying an edit whose original text no longer matches.
	replay, err := edit("key", "fp")
	require.NoError(t, err)
	require.Equal(t, first, replay)
	require.Equal(t, 1, adapter.applied)

	_, err = edit("key", "different")
	require.ErrorIs(t, err, agentrunonce.ErrInputMismatch)
	require.Equal(t, 1, adapter.applied)
}

func TestForgetIgnoresSupersededGeneration(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)
	start := time.Now()

	first, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)
	first.Ticket.Publish("first")

	stale, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)
	require.Equal(t, "first", stale.Value)

	// The reservation the stale observation came from is reaped and the
	// key reserved and published again.
	registry.Complete("key", start)
	require.Equal(t, []string{"first"}, registry.Reap(start.Add(testRetention).Add(time.Second)))
	second, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)
	require.NotNil(t, second.Ticket)
	second.Ticket.Publish("second")

	// Forgetting with the stale generation must not evict the newer
	// reservation, or its operation would run a second time.
	registry.Forget("key", stale.Generation)

	outcome, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)
	require.Nil(t, outcome.Ticket)
	require.Equal(t, "second", outcome.Value)

	registry.Forget("key", second.Generation)
	fresh, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)
	require.NotNil(t, fresh.Ticket)
}

func TestAwaitReportsUnreservedKey(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)

	_, err := registry.Await(context.Background(), "key")
	require.ErrorIs(t, err, agentrunonce.ErrNotReserved)
}

func TestAwaitReturnsPublishedValue(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)

	outcome, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)
	outcome.Ticket.Publish("value")

	value, err := registry.Await(context.Background(), "key")
	require.NoError(t, err)
	require.Equal(t, "value", value)
}

func TestAwaitWaitsForPendingReservation(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)

	outcome, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)

	awaited := make(chan string, 1)
	go func() {
		value, err := registry.Await(context.Background(), "key")
		if err == nil {
			awaited <- value
		}
	}()

	require.Never(t, func() bool {
		return len(awaited) > 0
	}, testutil.IntervalMedium, testutil.IntervalFast)

	outcome.Ticket.Publish("value")
	require.Equal(t, "value", testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, awaited))
}

func TestAwaitReportsPendingAtDeadline(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)

	_, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = registry.Await(ctx, "key")
	require.ErrorIs(t, err, agentrunonce.ErrPublicationPending)
	require.NotErrorIs(t, err, agentrunonce.ErrNotReserved)
}

func TestAwaitTreatsReleasedReservationAsUnreserved(t *testing.T) {
	t.Parallel()

	registry := agentrunonce.NewRegistry[string, string](testRetention)

	outcome, err := registry.Reserve(context.Background(), "key", "fp")
	require.NoError(t, err)

	awaited := make(chan error, 1)
	go func() {
		_, err := registry.Await(context.Background(), "key")
		awaited <- err
	}()

	outcome.Ticket.Release()
	require.ErrorIs(t, testutil.RequireReceive(testutil.Context(t, testutil.WaitShort), t, awaited), agentrunonce.ErrNotReserved)
}
