package nats //nolint:testpackage // Exercises internal pubsub state and helpers.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

func Test_pickConn(t *testing.T) {
	t.Parallel()

	t.Run("DifferentSubjects", func(t *testing.T) {
		t.Parallel()
		var a, b natsgo.Conn
		pool := []*natsgo.Conn{&a, &b}

		require.NotSame(t, pickConn(pool, "a"), pickConn(pool, "b"))
	})
}

func Test_New(t *testing.T) {
	t.Parallel()

	t.Run("ConnectionCount", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		ps, err := New(ctx, logger, Options{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = ps.Close() })

		const n = 50
		cancels := make([]func(), 0, n)
		for i := 0; i < n; i++ {
			c, err := ps.Subscribe(fmt.Sprintf("cc_evt_%d", i), func(context.Context, []byte) {})
			require.NoError(t, err)
			cancels = append(cancels, c)
		}
		t.Cleanup(func() {
			for _, c := range cancels {
				c()
			}
		})

		require.Equal(t, 2, ps.ns.NumClients(),
			"expected exactly 2 client connections (pubConn + subConn), got %d", ps.ns.NumClients())
		require.Len(t, ps.publishPool, 1, "default PublishConns must be 1")
		require.Len(t, ps.subscribePool, 1, "default SubscribeConns must be 1")
		require.NotSame(t, ps.publishPool[0], ps.subscribePool[0], "pubConn and subConn must be distinct")
	})
}

func Test_SubscribeWithErr(t *testing.T) {
	t.Parallel()

	t.Run("SameSubjectSharesSubscription", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		ps, err := New(ctx, logger, Options{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = ps.Close() })

		cancelA, err := ps.Subscribe("coalesce_evt", func(context.Context, []byte) {})
		require.NoError(t, err)
		t.Cleanup(cancelA)
		cancelB, err := ps.Subscribe("coalesce_evt", func(context.Context, []byte) {})
		require.NoError(t, err)
		t.Cleanup(cancelB)

		ps.mu.Lock()
		defer ps.mu.Unlock()
		require.Len(t, ps.subscriptions, 1)
	})
}

func Test_localSub_init(t *testing.T) {
	t.Parallel()

	t.Run("SerializesCallbacks", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		dataStarted := make(chan struct{})
		dropDelivered := make(chan struct{})
		release := make(chan struct{})
		var dataOnce sync.Once
		var dropOnce sync.Once
		var releaseOnce sync.Once
		var active atomic.Int64
		var concurrent atomic.Bool

		s := &localSub{
			ctx: ctx,
			listener: func(_ context.Context, _ []byte, ferr error) {
				if active.Add(1) != 1 {
					concurrent.Store(true)
				}
				defer active.Add(-1)

				if errors.Is(ferr, pubsub.ErrDroppedMessages) {
					dropOnce.Do(func() { close(dropDelivered) })
					return
				}

				dataOnce.Do(func() { close(dataStarted) })
				<-release
			},
			queue:          make(chan []byte, 1),
			dropSignal:     make(chan struct{}, 1),
			stop:           make(chan struct{}),
			dispatcherDone: make(chan struct{}),
		}
		s.init()
		t.Cleanup(func() {
			releaseOnce.Do(func() { close(release) })
			s.close()
		})

		s.enqueue([]byte("data"))
		require.Eventually(t, func() bool {
			select {
			case <-dataStarted:
				return true
			default:
				return false
			}
		}, testutil.WaitShort, testutil.IntervalFast)

		s.signalDrop()
		require.Never(t, func() bool {
			select {
			case <-dropDelivered:
				return true
			default:
				return false
			}
		}, testutil.IntervalMedium, testutil.IntervalFast,
			"drop callback must wait for the blocked data callback")
		require.False(t, concurrent.Load(), "listener callback ran concurrently")

		releaseOnce.Do(func() { close(release) })
		require.Eventually(t, func() bool {
			select {
			case <-dropDelivered:
				return true
			default:
				return false
			}
		}, testutil.WaitShort, testutil.IntervalFast)
		require.False(t, concurrent.Load(), "listener callback ran concurrently")
	})

	t.Run("SlowListenerIsolation", func(t *testing.T) {
		t.Parallel()
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		ps, err := New(ctx, logger, Options{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = ps.Close() })

		release := make(chan struct{})
		var releaseOnce sync.Once
		var slowDrops atomic.Int64
		var slowBlocked atomic.Bool
		slowCancel, err := ps.SubscribeWithErr("iso_slow", func(_ context.Context, _ []byte, ferr error) {
			if ferr != nil && errors.Is(ferr, pubsub.ErrDroppedMessages) {
				slowDrops.Add(1)
				return
			}
			if slowBlocked.CompareAndSwap(false, true) {
				<-release
			}
		})
		require.NoError(t, err)
		defer slowCancel()

		var fastCount atomic.Int64
		fastCancel, err := ps.Subscribe("iso_fast", func(_ context.Context, _ []byte) {
			fastCount.Add(1)
		})
		require.NoError(t, err)
		defer fastCancel()
		defer releaseOnce.Do(func() { close(release) })

		total := defaultListenerQueueSize + 256
		payload := make([]byte, 4*1024)
		for i := 0; i < total; i++ {
			require.NoError(t, ps.Publish("iso_slow", payload))
			require.NoError(t, ps.Publish("iso_fast", []byte("ping")))
		}
		require.NoError(t, ps.Flush())

		require.Eventually(t, func() bool {
			return fastCount.Load() >= int64(total)
		}, testutil.WaitLong, testutil.IntervalFast)
		require.Zero(t, slowDrops.Load(),
			"drop callback must wait for the blocked data callback")
		releaseOnce.Do(func() { close(release) })
		require.Eventually(t, func() bool {
			return slowDrops.Load() >= 1
		}, testutil.WaitLong, testutil.IntervalFast,
			"slow subscriber must receive at least one ErrDroppedMessages signal")

		require.GreaterOrEqual(t, fastCount.Load(), int64(total),
			"fast subscriber must keep receiving despite slow peer on shared subConn")
		require.Len(t, ps.subscribePool, 1)
		require.False(t, ps.subscribePool[0].IsClosed(), "subConn must not be closed by slow consumer")
		require.True(t, ps.subscribePool[0].IsConnected(), "subConn must stay connected")
		require.Equal(t, 2, ps.ns.NumClients(), "slow consumer must not disconnect subConn")
	})
}
