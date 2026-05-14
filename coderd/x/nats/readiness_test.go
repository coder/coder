//nolint:testpackage // Internal test: exercises sharedSub readiness internals.
package nats

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

// newReadinessPubsub builds a Pubsub for readiness tests. Each test
// gets its own embedded server so test hooks installed on p do not
// leak across the suite.
func newReadinessPubsub(t *testing.T) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
		Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })
	return ps
}

// TestReadiness_JoinerWaitsForCreator verifies that a second
// SubscribeWithErr for the same subject does not return until the
// first subscription's NATS Subscribe + Flush + SetPendingLimits
// sequence has completed. Without the readiness barrier, the joiner
// could return early and a publish issued immediately after could be
// lost by the server because the SUB has not yet been registered.
//
// We deterministically stall the creator inside the Flush window via
// testHookBeforeFlush and assert that the second Subscribe blocks.
func TestReadiness_JoinerWaitsForCreator(t *testing.T) {
	t.Parallel()
	ps := newReadinessPubsub(t)

	const event = "ready_joiner_evt"

	release := make(chan struct{})
	creatorAtHook := make(chan struct{})
	var hookFired atomic.Bool
	ps.testHookBeforeFlush = func(string) {
		if !hookFired.CompareAndSwap(false, true) {
			return
		}
		close(creatorAtHook)
		<-release
	}

	// Creator's Subscribe runs in a goroutine; it will block inside
	// the test hook before Flush completes.
	creatorDone := make(chan struct{})
	var creatorCancel func()
	var creatorErr error
	go func() {
		defer close(creatorDone)
		creatorCancel, creatorErr = ps.Subscribe(event, func(context.Context, []byte) {})
	}()
	// Wait until the creator is parked at the hook so the shared
	// entry exists and the joiner will take the joiner path.
	select {
	case <-creatorAtHook:
	case <-time.After(testutil.WaitShort):
		t.Fatal("creator never reached pre-Flush hook")
	}
	require.Equal(t, 1, sharedCount(ps),
		"creator must have inserted a shared placeholder before Flush")

	// Now fire the joiner. It must block on shared.ready until we
	// release the creator. We measure blocked-ness by racing a
	// short timeout against the joiner returning.
	joinerDone := make(chan error, 1)
	var joinerCancel atomic.Pointer[func()]
	go func() {
		c, err := ps.Subscribe(event, func(context.Context, []byte) {})
		if c != nil {
			joinerCancel.Store(&c)
		}
		joinerDone <- err
	}()
	select {
	case err := <-joinerDone:
		t.Fatalf("joiner returned (err=%v) before creator readiness; readiness barrier missing", err)
	case <-time.After(50 * time.Millisecond):
		// expected: joiner is blocked on shared.ready
	}

	// Release the creator. Both creator and joiner must now complete
	// successfully.
	close(release)
	select {
	case <-creatorDone:
	case <-time.After(testutil.WaitShort):
		t.Fatal("creator did not finish after release")
	}
	require.NoError(t, creatorErr)
	select {
	case err := <-joinerDone:
		require.NoError(t, err)
	case <-time.After(testutil.WaitShort):
		t.Fatal("joiner did not finish after release")
	}

	t.Cleanup(func() {
		if c := joinerCancel.Load(); c != nil {
			(*c)()
		}
		if creatorCancel != nil {
			creatorCancel()
		}
	})

	// Both attached to the same shared subscription.
	require.Equal(t, 1, sharedCount(ps))
	require.Equal(t, 2, listenerCount(ps))
}

// TestReadiness_PublishAfterJoinerNotLost is the end-to-end version
// of the readiness guarantee. It asserts that a Publish issued in the
// instant after a joiner's SubscribeWithErr returns is delivered to
// that joiner. Before the readiness barrier, the joiner could return
// before the creator's Flush had reached the server, and the publish
// would arrive at the server with no registered SUB.
func TestReadiness_PublishAfterJoinerNotLost(t *testing.T) {
	t.Parallel()
	ps := newReadinessPubsub(t)

	const event = "ready_publish_evt"

	// Stall the creator inside the Flush window. The joiner attaches
	// while the creator is parked; when we release, both proceed.
	release := make(chan struct{})
	creatorAtHook := make(chan struct{})
	var hookFired atomic.Bool
	ps.testHookBeforeFlush = func(string) {
		if !hookFired.CompareAndSwap(false, true) {
			return
		}
		close(creatorAtHook)
		<-release
	}

	gotCreator := make(chan []byte, 1)
	creatorReady := make(chan struct{})
	creatorErr := make(chan error, 1)
	go func() {
		c, err := ps.Subscribe(event, func(_ context.Context, msg []byte) {
			select {
			case gotCreator <- msg:
			default:
			}
		})
		if err == nil {
			t.Cleanup(c)
		}
		creatorErr <- err
		close(creatorReady)
	}()
	select {
	case <-creatorAtHook:
	case <-time.After(testutil.WaitShort):
		t.Fatal("creator did not reach hook")
	}

	// Start the joiner. It blocks on readiness.
	gotJoiner := make(chan []byte, 1)
	joinerReady := make(chan struct{})
	joinerErr := make(chan error, 1)
	go func() {
		c, err := ps.Subscribe(event, func(_ context.Context, msg []byte) {
			select {
			case gotJoiner <- msg:
			default:
			}
		})
		if err == nil {
			t.Cleanup(c)
		}
		joinerErr <- err
		close(joinerReady)
	}()

	// Release. Both subscriptions become ready. The joiner observes
	// `ready` only after the creator's Flush and SetPendingLimits
	// have completed; a Publish-after-return must be delivered.
	close(release)
	select {
	case <-creatorReady:
	case <-time.After(testutil.WaitShort):
		t.Fatal("creator never returned from Subscribe")
	}
	require.NoError(t, <-creatorErr)
	select {
	case <-joinerReady:
	case <-time.After(testutil.WaitShort):
		t.Fatal("joiner never returned from Subscribe")
	}
	require.NoError(t, <-joinerErr)

	require.NoError(t, ps.Publish(event, []byte("after-joiner")))
	require.NoError(t, ps.Flush())

	ctx := testutil.Context(t, testutil.WaitShort)
	cMsg := testutil.TryReceive(ctx, t, gotCreator)
	jMsg := testutil.TryReceive(ctx, t, gotJoiner)
	require.Equal(t, "after-joiner", string(cMsg))
	require.Equal(t, "after-joiner", string(jMsg),
		"joiner must observe publish issued after its SubscribeWithErr returned")
}

// TestReadiness_FlushFailureCleansUp asserts that when the creator's
// readiness sequence fails (we force a failure by closing subConn
// inside the hook), the shared entry is removed from the registries
// and the underlying NATS subscription is unsubscribed. No state
// must leak.
func TestReadiness_FlushFailureCleansUp(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
		Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })

	const event = "ready_flush_fail_evt"
	subject := subjectFor(t, event)

	// Capture the natsSub the wrapper handed to the failing creator
	// so we can assert it is no longer reachable from sharedByNATS
	// afterwards. We sniff the shared entry inside the hook (the
	// entry is registered before the hook fires).
	var inFlightShared *sharedSub
	ps.testHookBeforeFlush = func(subj string) {
		if subj != subject {
			return
		}
		ps.mu.Lock()
		inFlightShared = ps.sharedBySubject[subj]
		ps.mu.Unlock()
		// Force the upcoming Flush to fail by closing the subConn.
		// nats.go's Flush on a closed conn returns ErrConnectionClosed.
		ps.subConn.Close()
	}

	_, err = ps.Subscribe(event, func(context.Context, []byte) {})
	require.Error(t, err, "Flush must fail after we closed subConn")

	// No leaked registry state: sharedBySubject and sharedByNATS
	// must not contain the failed shared.
	require.Equal(t, 0, sharedCount(ps),
		"failed creator must remove the shared entry from sharedBySubject")
	require.Equal(t, 0, listenerCount(ps),
		"failed creator must remove its own listener from p.subs")
	require.NotNil(t, inFlightShared,
		"hook must have observed the in-flight shared")

	// The natsSub may or may not have been stored in sharedByNATS
	// yet (we set sharedByNATS only on the success path); what
	// matters is that after failure sharedByNATS does not contain
	// any entry pointing at the failed shared.
	ps.mu.Lock()
	for _, ss := range ps.sharedByNATS {
		require.NotSame(t, inFlightShared, ss,
			"failed creator must purge sharedByNATS")
	}
	ps.mu.Unlock()

	// The shared's underlying NATS subscription must be
	// unsubscribed by finishCreator. IsValid returns false after
	// Unsubscribe.
	require.NotNil(t, inFlightShared.sub,
		"natsSub must have been created before Flush was attempted")
	require.False(t, inFlightShared.sub.IsValid(),
		"failed creator must Unsubscribe the underlying *natsgo.Subscription")
}

// TestClose_DuringInitDoesNotDeadlock asserts that Close issued while
// a SubscribeWithErr is in its initialization window completes
// promptly and that the returning SubscribeWithErr does NOT report a
// successful, but stopped, subscription.
//
// We park the creator inside the Flush window, then run Close
// concurrently. Close must observe the in-flight listener in p.subs
// (the creator inserted s before unlocking), wait for its dispatcher
// goroutines (which were started before attach), and return. The
// creator's SubscribeWithErr then unblocks via the hook and must
// return an error rather than a usable subscription.
func TestClose_DuringInitDoesNotDeadlock(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
		Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{})
	require.NoError(t, err)

	const event = "close_during_init_evt"

	creatorAtHook := make(chan struct{})
	releaseCreator := make(chan struct{})
	var hookFired atomic.Bool
	ps.testHookBeforeFlush = func(string) {
		if !hookFired.CompareAndSwap(false, true) {
			return
		}
		close(creatorAtHook)
		<-releaseCreator
	}

	type result struct {
		cancel func()
		err    error
	}
	creatorResult := make(chan result, 1)
	go func() {
		c, err := ps.Subscribe(event, func(context.Context, []byte) {})
		creatorResult <- result{cancel: c, err: err}
	}()
	select {
	case <-creatorAtHook:
	case <-time.After(testutil.WaitShort):
		t.Fatal("creator did not reach pre-Flush hook")
	}

	// Close in a goroutine. It must not deadlock waiting on
	// dispatcher / emitter goroutines that may have been "registered
	// but not started". With the fix, listener goroutines are
	// started before registration, so Close can wait on them safely.
	closeDone := make(chan error, 1)
	go func() { closeDone <- ps.Close() }()

	// Wait deterministically for Close to have called p.cancel().
	// This is the moment after which SubscribeWithErr must observe
	// p.ctx.Err() != nil at its final guard and return an error
	// rather than a successful subscription. Polling p.ctx.Err()
	// directly is the test seam we have without adding more hooks.
	require.Eventually(t, func() bool { return ps.ctx.Err() != nil },
		testutil.WaitShort, testutil.IntervalFast,
		"Close must cancel p.ctx promptly")

	// While Close is in flight (its cancel has fired), let the
	// creator's Flush proceed. The Flush may or may not succeed
	// depending on whether subConn has been drained yet; either way
	// the SubscribeWithErr must not return a usable cancel function
	// for a closed Pubsub.
	close(releaseCreator)

	select {
	case err := <-closeDone:
		require.NoError(t, err, "Close must not error")
	case <-time.After(testutil.WaitMedium):
		t.Fatal("Close deadlocked during init window")
	}

	select {
	case r := <-creatorResult:
		require.Error(t, r.err,
			"SubscribeWithErr issued during Close window must not return a successful subscription")
		require.Nil(t, r.cancel,
			"errored SubscribeWithErr must not return a non-nil cancel func")
	case <-time.After(testutil.WaitShort):
		t.Fatal("SubscribeWithErr never returned after Close")
	}

	require.Equal(t, 0, sharedCount(ps))
	require.Equal(t, 0, listenerCount(ps))
}

// TestClose_RejectsNewSubscribes verifies that once Close has begun
// (p.ctx canceled), new SubscribeWithErr calls bail with an error
// rather than registering. This is the close-vs-Subscribe race
// resolution at the registration boundary.
func TestClose_RejectsNewSubscribes(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
		Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{})
	require.NoError(t, err)

	require.NoError(t, ps.Close())

	// After Close, every SubscribeWithErr / Subscribe must fail
	// before registering anything.
	_, err = ps.Subscribe("post_close_evt", func(context.Context, []byte) {})
	require.Error(t, err)
	require.Equal(t, 0, listenerCount(ps))
	require.Equal(t, 0, sharedCount(ps))
}

// TestReadiness_ManyConcurrentJoinersOneCreator stresses the joiner
// path: while a creator is parked inside its readiness window, many
// concurrent joiners attach. All must wake up after the creator
// completes and observe a fully-ready shared subscription; none must
// observe a half-initialized one.
func TestReadiness_ManyConcurrentJoinersOneCreator(t *testing.T) {
	t.Parallel()
	ps := newReadinessPubsub(t)

	const (
		event      = "ready_many_joiners_evt"
		numJoiners = 32
	)

	creatorAtHook := make(chan struct{})
	release := make(chan struct{})
	var hookFired atomic.Bool
	ps.testHookBeforeFlush = func(string) {
		if !hookFired.CompareAndSwap(false, true) {
			return
		}
		close(creatorAtHook)
		<-release
	}

	creatorReady := make(chan struct{})
	creatorErr := make(chan error, 1)
	go func() {
		c, err := ps.Subscribe(event, func(context.Context, []byte) {})
		if err == nil {
			t.Cleanup(c)
		}
		creatorErr <- err
		close(creatorReady)
	}()
	select {
	case <-creatorAtHook:
	case <-time.After(testutil.WaitShort):
		t.Fatal("creator never reached hook")
	}

	var wg sync.WaitGroup
	errs := make(chan error, numJoiners)
	cancels := make(chan func(), numJoiners)
	for i := 0; i < numJoiners; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := ps.Subscribe(event, func(context.Context, []byte) {})
			errs <- err
			cancels <- c
		}()
	}

	// Briefly assert none of the joiners returned yet.
	select {
	case err := <-errs:
		t.Fatalf("joiner returned (err=%v) before creator readiness", err)
	case <-time.After(testutil.IntervalFast):
		// expected
	}

	close(release)
	<-creatorReady
	require.NoError(t, <-creatorErr)
	wg.Wait()
	close(errs)
	close(cancels)
	for err := range errs {
		require.NoError(t, err, "every joiner must succeed once creator is ready")
	}
	cs := make([]func(), 0, numJoiners)
	for c := range cancels {
		cs = append(cs, c)
	}
	t.Cleanup(func() {
		for _, c := range cs {
			if c != nil {
				c()
			}
		}
	})

	require.Equal(t, 1, sharedCount(ps))
	require.Equal(t, numJoiners+1, listenerCount(ps))
}

// TestZeroCopy_FanOutPreservesPayloadIdentity documents and verifies
// the zero-copy contract: every coalesced listener for the same
// subject observes the SAME backing array on the same publish. We do
// not assert byte equality alone (that would not prove the contract);
// we compare the slice header data pointers via the captured slice
// value.
//
// This test exists so a future refactor that adds cloning is loud:
// the equality assertion will start failing, forcing a deliberate
// decision and an update of the contract documentation in
// pubsub.go's makeCallback comment.
func TestZeroCopy_FanOutPreservesPayloadIdentity(t *testing.T) {
	t.Parallel()
	ps := newReadinessPubsub(t)

	const event = "zero_copy_evt"

	gotA := make(chan []byte, 1)
	gotB := make(chan []byte, 1)
	cA, err := ps.Subscribe(event, func(_ context.Context, msg []byte) {
		select {
		case gotA <- msg:
		default:
		}
	})
	require.NoError(t, err)
	t.Cleanup(cA)
	cB, err := ps.Subscribe(event, func(_ context.Context, msg []byte) {
		select {
		case gotB <- msg:
		default:
		}
	})
	require.NoError(t, err)
	t.Cleanup(cB)

	require.NoError(t, ps.Publish(event, []byte("hello-zero-copy")))
	require.NoError(t, ps.Flush())

	ctx := testutil.Context(t, testutil.WaitShort)
	a := testutil.TryReceive(ctx, t, gotA)
	b := testutil.TryReceive(ctx, t, gotB)

	require.Equal(t, "hello-zero-copy", string(a))
	require.Equal(t, "hello-zero-copy", string(b))
	// Identity check: same backing array, no clone. If this fails,
	// someone added a copy somewhere in the fan-out path; either
	// restore the zero-copy fan-out or update the contract comment
	// in pubsub.go's makeCallback and remove this assertion.
	require.True(t, &a[0] == &b[0],
		"coalesced fan-out must deliver the SAME []byte backing array to every listener (zero-copy contract)")
}
