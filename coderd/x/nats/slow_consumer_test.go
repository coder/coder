package nats //nolint:testpackage // Uses internal symbols for white-box dedup testing.

import (
	"context"
	"errors"
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

func newSlowConsumerPubsub(t *testing.T) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{
		PendingLimits: PendingLimits{Msgs: 8, Bytes: 1024 * 1024},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })
	return ps
}

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

	ctx := testutil.Context(t, testutil.WaitLong)
	var dropCount, msgCount int
	var sawDrop bool
	deadline := time.After(testutil.WaitShort)
collect:
	for {
		select {
		case d := <-deliveries:
			if errors.Is(d.err, pubsub.ErrDroppedMessages) {
				dropCount++
				sawDrop = true
			} else if d.err == nil {
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
