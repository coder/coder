//nolint:testpackage // Internal test: inspects sharedBySubject to verify per-subject coalescing.
package nats

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

// newCoalescingPubsub builds a default-options Pubsub for white-box
// coalescing tests. Each test gets its own embedded server.
func newCoalescingPubsub(t *testing.T) *Pubsub {
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

// sharedCount returns the number of *natsgo.Subscription objects the
// Pubsub currently owns. Coalescing means this equals the number of
// distinct active subjects regardless of how many local subscribers
// each subject has.
func sharedCount(p *Pubsub) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.sharedBySubject)
}

// listenerCount returns the number of local listeners (Subscribe /
// SubscribeWithErr handles) currently registered.
func listenerCount(p *Pubsub) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.subs)
}

// listenerCountForSubject returns how many local listeners are
// attached to the shared subscription for subject.
func listenerCountForSubject(p *Pubsub, subject string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	ss, ok := p.sharedBySubject[subject]
	if !ok {
		return 0
	}
	return len(ss.listeners)
}

// subjectFor returns the NATS subject the wrapper publishes under for
// the given event name. Since the wrapper now uses event names directly
// as subjects, this is the identity function; it is kept to clarify
// intent at call sites.
func subjectFor(t *testing.T, event string) string {
	t.Helper()
	return event
}

// TestCoalescing_OneSubscriptionForManyLocalSubscribers verifies that N
// concurrent local subscribers on the same event share exactly one
// underlying *natsgo.Subscription. This is the headline regression
// guard for the same-subject coalescing change.
func TestCoalescing_OneSubscriptionForManyLocalSubscribers(t *testing.T) {
	t.Parallel()
	ps := newCoalescingPubsub(t)

	const (
		event   = "coalesce_evt"
		numSubs = 50
	)
	subject := subjectFor(t, event)

	cancels := make([]func(), 0, numSubs)
	counts := make([]atomic.Int64, numSubs)
	for i := 0; i < numSubs; i++ {
		i := i
		c, err := ps.Subscribe(event, func(_ context.Context, _ []byte) {
			counts[i].Add(1)
		})
		require.NoError(t, err)
		cancels = append(cancels, c)
	}
	t.Cleanup(func() {
		for _, c := range cancels {
			c()
		}
	})

	require.Equal(t, 1, sharedCount(ps),
		"%d local subscribers on one event must share one shared subscription", numSubs)
	require.Equal(t, numSubs, listenerCount(ps),
		"all local subscribers must be tracked")
	require.Equal(t, numSubs, listenerCountForSubject(ps, subject),
		"all local subscribers must be attached to the same shared sub")

	// And delivery still works: every local subscriber must receive
	// every published message.
	const numMsgs = 10
	for i := 0; i < numMsgs; i++ {
		require.NoError(t, ps.Publish(event, []byte("m")))
	}
	require.Eventually(t, func() bool {
		for i := 0; i < numSubs; i++ {
			if counts[i].Load() < int64(numMsgs) {
				return false
			}
		}
		return true
	}, testutil.WaitLong, testutil.IntervalFast,
		"every coalesced subscriber must receive every message")
}

// TestCoalescing_CancelOneKeepsOthers verifies that canceling one
// local subscriber on a shared event does not affect delivery to
// remaining subscribers on the same event.
func TestCoalescing_CancelOneKeepsOthers(t *testing.T) {
	t.Parallel()
	ps := newCoalescingPubsub(t)

	const event = "coalesce_cancel_evt"
	subject := subjectFor(t, event)

	gotA := make(chan []byte, 16)
	gotB := make(chan []byte, 16)
	gotC := make(chan []byte, 16)
	cancelA, err := ps.Subscribe(event, func(_ context.Context, msg []byte) { gotA <- msg })
	require.NoError(t, err)
	cancelB, err := ps.Subscribe(event, func(_ context.Context, msg []byte) { gotB <- msg })
	require.NoError(t, err)
	cancelC, err := ps.Subscribe(event, func(_ context.Context, msg []byte) { gotC <- msg })
	require.NoError(t, err)
	defer cancelB()
	defer cancelC()

	require.Equal(t, 1, sharedCount(ps))
	require.Equal(t, 3, listenerCountForSubject(ps, subject))

	// Cancel the middle subscriber.
	cancelA()

	require.Equal(t, 1, sharedCount(ps),
		"shared subscription must persist while other listeners remain")
	require.Equal(t, 2, listenerCountForSubject(ps, subject),
		"only the canceled listener should be detached")

	require.NoError(t, ps.Publish(event, []byte("after-cancel")))

	ctx := testutil.Context(t, testutil.WaitShort)
	bMsg := testutil.TryReceive(ctx, t, gotB)
	cMsg := testutil.TryReceive(ctx, t, gotC)
	assert.Equal(t, "after-cancel", string(bMsg))
	assert.Equal(t, "after-cancel", string(cMsg))

	// Canceled subscriber must not get the post-cancel message. Give
	// the system a brief moment to be unambiguous about this.
	select {
	case msg := <-gotA:
		t.Fatalf("canceled subscriber unexpectedly received %q", string(msg))
	case <-time.After(testutil.IntervalSlow):
	}
}

// TestCoalescing_CancelAllThenResubscribe verifies that after every
// local subscriber on a subject cancels, a later Subscribe creates a
// fresh underlying NATS subscription and continues to deliver.
func TestCoalescing_CancelAllThenResubscribe(t *testing.T) {
	t.Parallel()
	ps := newCoalescingPubsub(t)

	const event = "coalesce_resub_evt"
	subject := subjectFor(t, event)

	// Initial wave of subscribers, then cancel all.
	cancels := make([]func(), 0, 5)
	for i := 0; i < 5; i++ {
		c, err := ps.Subscribe(event, func(context.Context, []byte) {})
		require.NoError(t, err)
		cancels = append(cancels, c)
	}
	require.Equal(t, 1, sharedCount(ps))
	firstNATSSub := ps.sharedBySubject[subject].sub
	require.NotNil(t, firstNATSSub)

	for _, c := range cancels {
		c()
	}
	require.Equal(t, 0, sharedCount(ps),
		"shared subscription must be torn down after the last listener cancels")
	require.Equal(t, 0, listenerCount(ps))

	// Resubscribe; a fresh shared subscription must appear.
	got := make(chan []byte, 1)
	c, err := ps.Subscribe(event, func(_ context.Context, msg []byte) { got <- msg })
	require.NoError(t, err)
	defer c()

	require.Equal(t, 1, sharedCount(ps))
	secondNATSSub := ps.sharedBySubject[subject].sub
	require.NotSame(t, firstNATSSub, secondNATSSub,
		"resubscribe after full teardown must allocate a new *natsgo.Subscription")

	require.NoError(t, ps.Publish(event, []byte("after-resub")))
	ctx := testutil.Context(t, testutil.WaitShort)
	msg := testutil.TryReceive(ctx, t, got)
	assert.Equal(t, "after-resub", string(msg))
}

// TestCoalescing_DifferentEventsIsolated verifies that coalescing does
// not bleed messages across different events; each event still gets
// its own shared subscription with independent delivery.
func TestCoalescing_DifferentEventsIsolated(t *testing.T) {
	t.Parallel()
	ps := newCoalescingPubsub(t)

	gotA := make(chan []byte, 4)
	gotB := make(chan []byte, 4)
	cancelA, err := ps.Subscribe("evt_a", func(_ context.Context, msg []byte) { gotA <- msg })
	require.NoError(t, err)
	defer cancelA()
	cancelB, err := ps.Subscribe("evt_b", func(_ context.Context, msg []byte) { gotB <- msg })
	require.NoError(t, err)
	defer cancelB()

	require.Equal(t, 2, sharedCount(ps),
		"distinct subjects must each have their own shared subscription")

	require.NoError(t, ps.Publish("evt_a", []byte("to-a")))
	require.NoError(t, ps.Publish("evt_b", []byte("to-b")))

	ctx := testutil.Context(t, testutil.WaitShort)
	aMsg := testutil.TryReceive(ctx, t, gotA)
	bMsg := testutil.TryReceive(ctx, t, gotB)
	assert.Equal(t, "to-a", string(aMsg))
	assert.Equal(t, "to-b", string(bMsg))

	// Cross-talk check: neither channel may have an additional message.
	select {
	case msg := <-gotA:
		t.Fatalf("evt_a subscriber received unexpected message %q", string(msg))
	case msg := <-gotB:
		t.Fatalf("evt_b subscriber received unexpected message %q", string(msg))
	case <-time.After(testutil.IntervalSlow):
	}
}

// TestCoalescing_SlowLocalListenerIsolated verifies that with two
// local subscribers on the same coalesced subject, blocking one of
// them does not block the other. Both share a single underlying NATS
// subscription, but per-listener inboxes preserve cross-listener
// isolation.
func TestCoalescing_SlowLocalListenerIsolated(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
		Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })

	const event = "coalesce_slow_evt"

	release := make(chan struct{})
	// LIFO defer order: close(release) runs before the cancels so a
	// failed Eventually call cannot wedge cleanup on the parked
	// listener.
	var releaseOnce sync.Once
	doRelease := func() { releaseOnce.Do(func() { close(release) }) }

	var slowStarted atomic.Bool
	var slowDrops atomic.Int64
	slowCancel, err := ps.SubscribeWithErr(event, func(_ context.Context, _ []byte, err error) {
		if err != nil {
			if errors.Is(err, pubsub.ErrDroppedMessages) {
				slowDrops.Add(1)
			}
			return
		}
		if slowStarted.CompareAndSwap(false, true) {
			<-release
		}
	})
	require.NoError(t, err)
	defer slowCancel()

	var fastCount atomic.Int64
	fastCancel, err := ps.Subscribe(event, func(_ context.Context, _ []byte) {
		fastCount.Add(1)
	})
	require.NoError(t, err)
	defer fastCancel()
	// Defer release last so it runs first (LIFO) on test exit.
	defer doRelease()

	require.Equal(t, 1, sharedCount(ps),
		"two listeners on the same event must share one underlying subscription")

	// Publish more messages than the default per-listener queue capacity
	// so the slow listener's inbox unambiguously overflows while the
	// fast listener (whose dispatcher is not parked) drains every
	// message it receives.
	total := defaultListenerQueueSize + 256
	for i := 0; i < total; i++ {
		require.NoError(t, ps.Publish(event, []byte("payload")))
	}
	require.NoError(t, ps.Flush())

	// Fast listener must reach the total regardless of the slow
	// listener being parked, and the slow listener must observe at
	// least one ErrDroppedMessages callback because its inbox
	// overflowed.
	require.Eventually(t, func() bool {
		return fastCount.Load() >= int64(total) && slowDrops.Load() >= 1
	}, testutil.WaitLong, testutil.IntervalFast,
		"fast=%d slow_drops=%d", fastCount.Load(), slowDrops.Load())

	doRelease()
}

// TestCoalescing_CloseTerminatesAllDispatchers asserts that Close on a
// Pubsub with many coalesced subscribers returns promptly and that
// every local subscriber's dispatcher and emitter goroutines exit. We
// detect goroutine exit by waiting on subscription's dispatcherDone
// and emitterDone channels; both must be closed after Close returns.
func TestCoalescing_CloseTerminatesAllDispatchers(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
		Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{})
	require.NoError(t, err)

	const (
		event   = "coalesce_close_evt"
		numSubs = 20
	)

	var wg sync.WaitGroup
	wg.Add(numSubs)
	subs := make([]*subscription, 0, numSubs)
	for i := 0; i < numSubs; i++ {
		_, err := ps.SubscribeWithErr(event, func(context.Context, []byte, error) {
			// Listener body is intentionally trivial; we are
			// verifying lifecycle, not delivery.
		})
		require.NoError(t, err)
	}
	ps.mu.Lock()
	for s := range ps.subs {
		subs = append(subs, s)
	}
	ps.mu.Unlock()
	require.Len(t, subs, numSubs)
	require.Equal(t, 1, sharedCount(ps))

	closeStart := time.Now()
	closeDone := make(chan error, 1)
	go func() { closeDone <- ps.Close() }()
	select {
	case err := <-closeDone:
		require.NoError(t, err)
	case <-time.After(testutil.WaitMedium):
		t.Fatalf("Close did not return within %s", testutil.WaitMedium)
	}
	t.Logf("Close with %d coalesced subscribers took %s", numSubs, time.Since(closeStart))

	for i, s := range subs {
		select {
		case <-s.dispatcherDone:
		default:
			t.Fatalf("subscriber %d dispatcher goroutine did not exit after Close", i)
		}
		select {
		case <-s.emitterDone:
		default:
			t.Fatalf("subscriber %d drop emitter goroutine did not exit after Close", i)
		}
		// Account for wg so the test does not appear to leak.
		wg.Done()
	}
	// wg.Wait is purely a structural check; we already verified above.
	wg.Wait()
	require.Equal(t, 0, sharedCount(ps))
	require.Equal(t, 0, listenerCount(ps))
}

// TestCoalescing_ConcurrentSubscribeSameSubject stresses the
// attach/detach paths: many goroutines subscribe and cancel
// simultaneously on the same subject, mixed with publishes. The
// invariant we check at the end is that the wrapper has no leaked
// shared subscriptions and that publishes still reach a fresh
// subscriber.
func TestCoalescing_ConcurrentSubscribeSameSubject(t *testing.T) {
	t.Parallel()
	ps := newCoalescingPubsub(t)

	const (
		event      = "coalesce_concurrent_evt"
		numWorkers = 16
		iterations = 50
	)

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				c, err := ps.SubscribeWithErr(event, func(context.Context, []byte, error) {})
				if err != nil {
					return
				}
				if err := ps.Publish(event, []byte("x")); err != nil {
					c()
					return
				}
				c()
			}
		}()
	}

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()
	select {
	case <-doneCh:
	case <-time.After(testutil.WaitLong):
		t.Fatal("concurrent subscribers did not finish")
	}

	require.Equal(t, 0, sharedCount(ps),
		"after every worker cancels, no shared subscription must linger")
	require.Equal(t, 0, listenerCount(ps))

	// Flush the pub connection so any "x" payloads still buffered
	// inside nats.go drain to the server before we install the
	// post-storm subscriber; otherwise late stragglers could be
	// delivered to the new subscriber and clobber the "after-storm"
	// expectation below.
	require.NoError(t, ps.Flush())

	// A fresh subscription on the same subject must still receive
	// new publishes. Use a buffered channel and accept either "x"
	// stragglers or "after-storm": the assertion is that we receive
	// "after-storm" at all, not that it is the very first message.
	got := make(chan []byte, 64)
	c, err := ps.Subscribe(event, func(_ context.Context, msg []byte) {
		select {
		case got <- msg:
		default:
		}
	})
	require.NoError(t, err)
	defer c()
	require.NoError(t, ps.Publish(event, []byte("after-storm")))
	require.NoError(t, ps.Flush())

	ctx := testutil.Context(t, testutil.WaitShort)
	deadline := time.After(testutil.WaitShort)
	for {
		select {
		case msg := <-got:
			if string(msg) == "after-storm" {
				return
			}
		case <-deadline:
			t.Fatal("did not receive after-storm marker on fresh subscription")
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}

// TestCoalescing_PublishStrideAcrossSubjects sanity-checks that with
// many subjects, each gets exactly one shared subscription, even
// under interleaved subscribe / publish patterns.
func TestCoalescing_PublishStrideAcrossSubjects(t *testing.T) {
	t.Parallel()
	ps := newCoalescingPubsub(t)

	const numEvents = 8
	events := make([]string, numEvents)
	cancels := make([]func(), 0, numEvents)
	gots := make([]chan []byte, numEvents)
	for i := 0; i < numEvents; i++ {
		events[i] = fmt.Sprintf("stride_%d", i)
		gots[i] = make(chan []byte, 4)
		i := i
		c, err := ps.Subscribe(events[i], func(_ context.Context, msg []byte) {
			gots[i] <- msg
		})
		require.NoError(t, err)
		cancels = append(cancels, c)
	}
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	require.Equal(t, numEvents, sharedCount(ps))

	for i, evt := range events {
		require.NoError(t, ps.Publish(evt, []byte(fmt.Sprintf("v%d", i))))
	}

	ctx := testutil.Context(t, testutil.WaitShort)
	for i := range events {
		msg := testutil.TryReceive(ctx, t, gots[i])
		assert.Equal(t, fmt.Sprintf("v%d", i), string(msg))
	}
}
