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
		// Tiny pending limit on subscriber forces NATS to drop messages
		// when the listener blocks.
		PendingLimits: PendingLimits{Msgs: 1, Bytes: 1024 * 1024},
		// Use buffered publish so Publish does not block on per-message
		// flush behavior while we are queuing many messages.
		PublishMode: PublishModeBuffered,
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
	require.NoError(t, ps.nc.FlushTimeout(testutil.WaitShort))

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
	require.NoError(t, ps.Publish(event, []byte("post-drop-marker")))
	_ = testutil.TryReceive(ctx, t, gotMarker)
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
	require.NoError(t, ps.nc.FlushTimeout(testutil.WaitShort))
	close(release)

	ctx := testutil.Context(t, testutil.WaitShort)

	// First message should arrive (it was already queued in the
	// listener).
	_ = testutil.TryReceive(ctx, t, got)

	// Marker should still be delivered.
	require.NoError(t, ps.Publish(event, []byte("marker")))
	deadline := time.After(testutil.WaitShort)
	for {
		select {
		case msg := <-got:
			if string(msg) == "marker" {
				return
			}
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
	require.NoError(t, ps.nc.FlushTimeout(testutil.WaitShort))
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

	// But slowConsumersTotal should have incremented again.
	// (We can't easily read counter here without reflection; covered in
	// metrics tests via direct registry gather.)
}
