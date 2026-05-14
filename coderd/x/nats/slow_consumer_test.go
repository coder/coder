package nats //nolint:testpackage // Uses internal symbols for white-box dedup testing.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func newSlowConsumerPubsub(t *testing.T) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{
		// Tight pending limit so the parked listener overflows the
		// per-listener inbox quickly while still leaving enough head
		// room for a post-burst "marker" publish to enqueue without
		// racing the dispatcher under the race detector.
		PendingLimits: PendingLimits{Msgs: 8, Bytes: 1024 * 1024},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })
	return ps
}

// TestSlowConsumer_DropSignal_Sync exercises the sync NextMsg slow-consumer
// path: a slow listener should receive exactly one ErrDroppedMessages
// callback for the first burst, and a subsequent normal message must
// still be delivered.
func TestSlowConsumer_DropSignal_Sync(t *testing.T) {
	t.Parallel()
	ps := newSlowConsumerPubsub(t)

	const event = "slow_evt_sync"

	type delivery struct {
		msg []byte
		err error
	}
	deliveries := make(chan delivery, 64)
	release := make(chan struct{})
	var blocked atomic.Bool

	cancel, err := ps.SubscribeWithErr(event, func(_ context.Context, msg []byte, err error) {
		if !blocked.Swap(true) {
			// Block the very first invocation so subsequent publishes
			// fill the per-subscription pending queue and trip the
			// slow-consumer protection.
			<-release
		}
		deliveries <- delivery{msg: msg, err: err}
	})
	require.NoError(t, err)
	defer cancel()

	// Publish enough messages to exceed pending limit (1 msg).
	for i := 0; i < 50; i++ {
		require.NoError(t, ps.Publish(event, []byte("burst")))
	}
	// Force flush so the embedded server actually delivers.
	require.NoError(t, ps.Flush())

	// Release the listener.
	close(release)

	ctx := testutil.Context(t, testutil.WaitLong)

	var dropCount, msgCount int
	var sawDrop bool
	// We expect at least: the initial "burst" message delivered to the
	// blocked listener, one ErrDroppedMessages callback, and possibly
	// more bursts if not all were dropped.
	deadline := time.After(testutil.WaitShort)
collect:
	for {
		select {
		case d := <-deliveries:
			if d.err != nil {
				if errors.Is(d.err, pubsub.ErrDroppedMessages) {
					dropCount++
					sawDrop = true
				}
			} else {
				msgCount++
			}
		case <-deadline:
			break collect
		case <-ctx.Done():
			break collect
		}
	}

	assert.True(t, sawDrop, "expected at least one ErrDroppedMessages callback")
	assert.GreaterOrEqual(t, dropCount, 1, "expected at least one drop callback")
	assert.GreaterOrEqual(t, msgCount, 1, "expected at least the first message delivered")

	// After drops, the subscription should still deliver new publishes.
	gotMarker := make(chan struct{}, 1)
	cancel2, err := ps.SubscribeWithErr(event, func(_ context.Context, msg []byte, _ error) {
		if string(msg) == "post-drop-marker" {
			select {
			case gotMarker <- struct{}{}:
			default:
			}
		}
	})
	require.NoError(t, err)
	defer cancel2()
	// Retry the marker on a fast tick so a single in-flight publish
	// that happens to race the post-burst dispatcher drain cannot
	// fail the assertion under the race detector.
	markerTick := time.NewTicker(testutil.IntervalMedium)
	defer markerTick.Stop()
	require.NoError(t, ps.Publish(event, []byte("post-drop-marker")))
	for {
		select {
		case <-gotMarker:
			return
		case <-markerTick.C:
			require.NoError(t, ps.Publish(event, []byte("post-drop-marker")))
		case <-ctx.Done():
			t.Fatalf("did not receive post-drop-marker: %v", ctx.Err())
		}
	}
}

// TestSlowConsumer_PlainSubscribeNoErrCallback ensures that the plain
// Subscribe API silently swallows ErrDroppedMessages and that subsequent
// messages can still be delivered without panicking.
func TestSlowConsumer_PlainSubscribeNoErrCallback(t *testing.T) {
	t.Parallel()
	ps := newSlowConsumerPubsub(t)

	const event = "slow_evt_plain"

	got := make(chan []byte, 64)
	release := make(chan struct{})
	var blocked atomic.Bool

	cancel, err := ps.Subscribe(event, func(_ context.Context, msg []byte) {
		if !blocked.Swap(true) {
			<-release
		}
		select {
		case got <- msg:
		default:
		}
	})
	require.NoError(t, err)
	defer cancel()

	for i := 0; i < 50; i++ {
		require.NoError(t, ps.Publish(event, []byte("burst")))
	}
	require.NoError(t, ps.Flush())
	close(release)

	ctx := testutil.Context(t, testutil.WaitShort)

	// First message should arrive (it was already queued in the
	// listener).
	_ = testutil.TryReceive(ctx, t, got)

	// Marker should still be delivered. With same-subject coalescing
	// the per-listener inbox is bounded by Options.PendingLimits, and
	// the post-burst dispatcher race against marker enqueue is small
	// but real under -race; retry the publish on a fast tick until
	// the marker round-trips through the listener.
	markerSent := time.NewTicker(testutil.IntervalMedium)
	defer markerSent.Stop()
	require.NoError(t, ps.Publish(event, []byte("marker")))
	deadline := time.After(testutil.WaitShort)
	for {
		select {
		case msg := <-got:
			if string(msg) == "marker" {
				return
			}
		case <-markerSent.C:
			// Re-publish to absorb local-queue-overflow drops that
			// can swallow a single in-flight marker.
			require.NoError(t, ps.Publish(event, []byte("marker")))
		case <-deadline:
			t.Fatal("did not receive post-drop marker")
		}
	}
}

// TestSlowConsumer_Dedup verifies that two slow-consumer signals with no
// new dropped messages between them only emit a single
// ErrDroppedMessages callback.
func TestSlowConsumer_Dedup(t *testing.T) {
	t.Parallel()
	ps := newSlowConsumerPubsub(t)

	const event = "slow_evt_dedup"

	type delivery struct {
		msg []byte
		err error
	}
	deliveries := make(chan delivery, 64)
	release := make(chan struct{})
	var blocked atomic.Bool

	cancel, err := ps.SubscribeWithErr(event, func(_ context.Context, msg []byte, err error) {
		if !blocked.Swap(true) {
			<-release
		}
		deliveries <- delivery{msg: msg, err: err}
	})
	require.NoError(t, err)
	defer cancel()

	for i := 0; i < 50; i++ {
		require.NoError(t, ps.Publish(event, []byte("burst")))
	}
	require.NoError(t, ps.Flush())
	close(release)

	// Drain the channel and count drops.
	dropCount := 0
	deadline := time.After(testutil.WaitShort)
drain:
	for {
		select {
		case d := <-deliveries:
			if d.err != nil && errors.Is(d.err, pubsub.ErrDroppedMessages) {
				dropCount++
			}
		case <-deadline:
			break drain
		}
	}
	require.GreaterOrEqual(t, dropCount, 1, "expected initial drop callback")

	// Now manually invoke the async slow-consumer path with no new drops.
	// Find the tracked subscription via white-box access.
	ps.mu.Lock()
	require.Len(t, ps.subs, 1)
	var s *subscription
	for sub := range ps.subs {
		s = sub
	}
	ps.mu.Unlock()
	require.NotNil(t, s)

	// Wait until the async errH path on this conn has synced
	// s.lastDropped up to NATS's cumulative Dropped() count, and the
	// dropped count itself has stopped growing. The async errH
	// callback is dispatched from the conn's internal errchan
	// goroutine, so there is no synchronous way to flush pending
	// callbacks; poll until things settle.
	// Force sync lastDropped to current Dropped() by invoking
	// handleSlowConsumer with the current count. After this any further
	// calls with no new drops must emit no callback.
	ps.handleSlowConsumer(s)
	// Wait briefly for Dropped() to stabilize after the burst settles.
	last, derr := s.sub.Dropped()
	require.NoError(t, derr)
	require.Eventually(t, func() bool {
		cur, derr := s.sub.Dropped()
		if derr != nil {
			return false
		}
		if cur != last {
			last = cur
			return false
		}
		return true
	}, testutil.WaitShort, testutil.IntervalFast, "Dropped() never stabilized")
	// Re-sync after stabilization so lastDropped == final Dropped().
	ps.handleSlowConsumer(s)

	// Drain any drop callbacks that arrived during the stabilization
	// wait so the post-manual check sees a clean channel. The
	// coalesced wrapper has two drop emission paths (NATS-level
	// slow-consumer and per-listener inbox overflow). The inbox
	// overflow path runs on an independent emitter goroutine, so we
	// also drop any pending entry off s.dropSignal and then drain
	// deliveries until a quiet period elapses; otherwise an in-flight
	// emitter callback could race the post-handleAsync check below.
drainSignal:
	for {
		select {
		case <-s.dropSignal:
		default:
			break drainSignal
		}
	}
	drainDeadline := time.After(testutil.IntervalSlow)
drainDeliveries:
	for {
		select {
		case <-deliveries:
			// Reset the quiet timer: a late emitter callback may
			// still be in flight.
			drainDeadline = time.After(testutil.IntervalSlow)
		case <-drainDeadline:
			break drainDeliveries
		}
	}

	ps.handleAsyncError(s.sub, natsgo.ErrSlowConsumer)

	// No additional drop callback should be emitted (delta == 0).
	select {
	case d := <-deliveries:
		if d.err != nil && errors.Is(d.err, pubsub.ErrDroppedMessages) {
			t.Fatalf("expected no duplicate drop callback, got one")
		}
	case <-time.After(testutil.IntervalSlow):
		// good: nothing delivered
	}
}
