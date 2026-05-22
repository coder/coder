package nats_test

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
	xnats "github.com/coder/coder/v2/coderd/x/nats"
	"github.com/coder/coder/v2/testutil"
)

func newTestPubsub(t *testing.T, opts xnats.Options) *xnats.Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithCancel(context.Background())
	ps, err := xnats.New(ctx, logger, opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ps.Close()
		cancel()
	})
	return ps
}

func TestPubsub(t *testing.T) {
	t.Parallel()

	t.Run("RoundTrip", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, xnats.Options{})

		got := make(chan []byte, 1)
		cancel, err := ps.Subscribe("test_event", func(_ context.Context, msg []byte) {
			got <- msg
		})
		require.NoError(t, err)
		defer cancel()

		require.NoError(t, ps.Publish("test_event", []byte("hello")))

		select {
		case msg := <-got:
			assert.Equal(t, "hello", string(msg))
		case <-time.After(testutil.WaitShort):
			t.Fatal("timed out waiting for message")
		}
	})

	t.Run("SubscribeWithErrNormalMessage", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, xnats.Options{})

		got := make(chan []byte, 1)
		cancel, err := ps.SubscribeWithErr("evt", func(_ context.Context, msg []byte, err error) {
			assert.NoError(t, err)
			got <- msg
		})
		require.NoError(t, err)
		defer cancel()

		require.NoError(t, ps.Publish("evt", []byte("payload")))

		select {
		case msg := <-got:
			assert.Equal(t, "payload", string(msg))
		case <-time.After(testutil.WaitShort):
			t.Fatal("timed out waiting for message")
		}
	})

	t.Run("EchoDefault", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, xnats.Options{})

		got := make(chan []byte, 1)
		cancel, err := ps.Subscribe("echo_evt", func(_ context.Context, msg []byte) {
			got <- msg
		})
		require.NoError(t, err)
		defer cancel()

		require.NoError(t, ps.Publish("echo_evt", []byte("data")))

		select {
		case msg := <-got:
			assert.Equal(t, "data", string(msg))
		case <-time.After(testutil.WaitShort):
			t.Fatal("default should echo own messages")
		}
	})

	t.Run("Ordering", func(t *testing.T) {
		t.Parallel()
		ps := newTestPubsub(t, xnats.Options{})

		const n = 100
		got := make(chan []byte, n)
		cancel, err := ps.Subscribe("ord_evt", func(_ context.Context, msg []byte) {
			got <- msg
		})
		require.NoError(t, err)
		defer cancel()

		for i := 0; i < n; i++ {
			require.NoError(t, ps.Publish("ord_evt", []byte(fmt.Sprintf("%d", i))))
		}

		deadline := time.After(testutil.WaitLong)
		for i := 0; i < n; i++ {
			select {
			case msg := <-got:
				assert.Equal(t, fmt.Sprintf("%d", i), string(msg))
			case <-deadline:
				t.Fatalf("timed out at message %d/%d", i, n)
			}
		}
	})

	t.Run("CloseIdempotent", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		ps, err := xnats.New(ctx, logger, xnats.Options{})
		require.NoError(t, err)

		var first, second error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			first = ps.Close()
		}()
		wg.Wait()
		second = ps.Close()
		assert.NoError(t, first)
		assert.NoError(t, second)
	})

	t.Run("SlowConsumerDropSignal", func(t *testing.T) {
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
	})
}

func newSlowConsumerPubsub(t *testing.T) *xnats.Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithCancel(context.Background())
	ps, err := xnats.New(ctx, logger, xnats.Options{
		PendingLimits: xnats.PendingLimits{Msgs: 8, Bytes: 1024 * 1024},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = ps.Close()
		cancel()
	})
	return ps
}
