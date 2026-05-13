package nats //nolint:testpackage // Uses internal fields for dual-conn assertions.

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/testutil"
)

// TestDualConn_ConnectionCount verifies that N subscriptions on a single
// Pubsub yield exactly two client connections at the embedded server:
// pubConn and subConn. Subscription count must not affect connection
// count.
func TestDualConn_ConnectionCount(t *testing.T) {
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

	// Pubsub's two TCP-loopback connections must be the only clients the
	// embedded server reports, independent of subscription count.
	require.Equal(t, 2, ps.ns.NumClients(),
		"expected exactly 2 client connections (pubConn + subConn), got %d", ps.ns.NumClients())
	require.NotSame(t, ps.pubConn, ps.subConn, "pubConn and subConn must be distinct")
}

// TestDualConn_SlowListenerIsolation verifies that when one subscription's
// listener blocks long enough to trip its client-side PendingLimits, only
// that subscription receives ErrSlowConsumer / ErrDroppedMessages.
// Subscriptions on other subjects, multiplexed over the same subConn,
// keep receiving messages.
func TestDualConn_SlowListenerIsolation(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	// Leave Options.PendingLimits at the package default (effectively
	// unlimited) so the fast subscription cannot be tripped. The slow
	// subscription gets a tight per-sub limit applied directly below.
	ps, err := New(ctx, logger, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })

	release := make(chan struct{})
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

	// Tighten the slow sub's pending limits via white-box access so
	// only it trips slow-consumer. The fast sub keeps the package
	// default (effectively unlimited).
	ps.mu.Lock()
	for s := range ps.subs {
		if s.event == "iso_slow" {
			require.NoError(t, s.sub.SetPendingLimits(10, 64*1024))
		}
	}
	ps.mu.Unlock()

	var fastCount atomic.Int64
	fastCancel, err := ps.Subscribe("iso_fast", func(_ context.Context, _ []byte) {
		fastCount.Add(1)
	})
	require.NoError(t, err)
	defer fastCancel()

	// Stuff the slow subscription far past its PendingLimits so the
	// async error handler fires reliably; meanwhile publish to the
	// fast subject so we can confirm deliveries continue.
	const total = 200
	payload := make([]byte, 4*1024)
	for i := 0; i < total; i++ {
		require.NoError(t, ps.Publish("iso_slow", payload))
		require.NoError(t, ps.Publish("iso_fast", []byte("ping")))
	}
	require.NoError(t, ps.pubConn.FlushTimeout(testutil.WaitShort))

	deadline := time.Now().Add(testutil.WaitLong)
	for time.Now().Before(deadline) {
		if fastCount.Load() >= total && slowDrops.Load() >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	close(release)

	require.GreaterOrEqual(t, slowDrops.Load(), int64(1),
		"slow subscriber must receive at least one ErrDroppedMessages async signal")
	require.GreaterOrEqual(t, fastCount.Load(), int64(total),
		"fast subscriber must keep receiving despite slow peer on shared subConn")

	// subConn must stay connected throughout: the slow-consumer signal
	// is per-subscription, not per-conn.
	require.False(t, ps.subConn.IsClosed(), "subConn must not be closed by slow consumer")
	require.True(t, ps.subConn.IsConnected(), "subConn must stay connected")
	// Connection count must still be exactly 2.
	require.Equal(t, 2, ps.ns.NumClients(), "slow consumer must not disconnect subConn")
}
