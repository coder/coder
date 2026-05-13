package nats //nolint:testpackage // Uses internal fields for per-sub-conn white-box assertions.

import (
	"context"
	"errors"
	"fmt"
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

// newInternalPubsub constructs a *Pubsub from within the package so tests
// can read internal fields (subs, pubConn).
func newInternalPubsub(t *testing.T, opts Options) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, opts)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })
	return ps
}

// TestPerSubConn_Distinct verifies that two subscriptions on the same
// wrapper own different *nats.Conn pointers.
func TestPerSubConn_Distinct(t *testing.T) {
	t.Parallel()
	ps := newInternalPubsub(t, Options{})

	c1, err := ps.Subscribe("evt_a", func(context.Context, []byte) {})
	require.NoError(t, err)
	t.Cleanup(c1)
	c2, err := ps.Subscribe("evt_b", func(context.Context, []byte) {})
	require.NoError(t, err)
	t.Cleanup(c2)

	ps.mu.Lock()
	defer ps.mu.Unlock()
	require.Len(t, ps.subs, 2)
	seen := make(map[string]int)
	for s := range ps.subs {
		require.NotNil(t, s.nc, "subscription should own a dedicated conn")
		require.NotSame(t, ps.pubConn, s.nc, "sub conn must differ from pubConn")
		seen[fmt.Sprintf("%p", s.nc)]++
	}
	require.Len(t, seen, 2, "subscriptions must hold distinct *nats.Conn pointers")
}

// TestPerSubConn_CancelClosesConn verifies that canceling a subscription
// closes its dedicated connection.
func TestPerSubConn_CancelClosesConn(t *testing.T) {
	t.Parallel()
	ps := newInternalPubsub(t, Options{})

	cancel, err := ps.Subscribe("c_evt", func(context.Context, []byte) {})
	require.NoError(t, err)

	ps.mu.Lock()
	require.Len(t, ps.subs, 1)
	var s *subscription
	for sub := range ps.subs {
		s = sub
	}
	ps.mu.Unlock()
	require.NotNil(t, s)
	require.NotNil(t, s.nc)
	require.False(t, s.nc.IsClosed())

	nc := s.nc
	cancel()

	// Wait briefly for the ClosedHandler to fire; nats.go closes
	// asynchronously.
	deadline := time.Now().Add(testutil.WaitShort)
	for time.Now().Before(deadline) {
		if nc.IsClosed() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.True(t, nc.IsClosed(), "subscription conn should be closed after cancel")
}

// TestPerSubConn_CloseDrainsAll verifies Close shuts down every per-sub
// conn plus pubConn and is idempotent.
func TestPerSubConn_CloseDrainsAll(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{})
	require.NoError(t, err)

	const n = 8
	for i := 0; i < n; i++ {
		cancelFn, err := ps.Subscribe(fmt.Sprintf("close_evt_%d", i), func(context.Context, []byte) {})
		require.NoError(t, err)
		_ = cancelFn // do not cancel; Close should handle teardown
	}

	type closable interface{ IsClosed() bool }
	conns := make([]closable, 0, n)
	ps.mu.Lock()
	for s := range ps.subs {
		require.NotNil(t, s.nc)
		conns = append(conns, s.nc)
	}
	pub := ps.pubConn
	ps.mu.Unlock()
	require.Len(t, conns, n)
	require.False(t, pub.IsClosed())

	require.NoError(t, ps.Close())

	for i, nc := range conns {
		assert.Truef(t, nc.IsClosed(), "sub conn %d not closed after Close", i)
	}
	assert.True(t, pub.IsClosed(), "pubConn not closed after Close")

	// Idempotent.
	assert.NoError(t, ps.Close())
}

// TestPerSubConn_SlowConsumerIsolation verifies that a blocked listener
// on one subscription drops messages locally without disrupting another
// subscription on the same subject.
func TestPerSubConn_SlowConsumerIsolation(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
	defer cancel()
	// Tight bytes budget so the blocked subscription trips drops fast;
	// the healthy subscriber drains immediately and won't approach it.
	ps, err := New(ctx, logger, Options{
		PendingLimits: PendingLimits{Msgs: -1, Bytes: 128 * 1024},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })

	release := make(chan struct{})
	var slowDrops atomic.Int64
	var slowBlocked atomic.Bool
	slowCancel, err := ps.SubscribeWithErr("iso_evt", func(_ context.Context, _ []byte, ferr error) {
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
	fastCancel, err := ps.Subscribe("iso_evt", func(_ context.Context, _ []byte) {
		fastCount.Add(1)
	})
	require.NoError(t, err)
	defer fastCancel()

	// Each publish carries 32 KiB; blocked subscriber's 128 KiB pending
	// budget fits ~4 messages before drops start.
	const total = 200
	payload := make([]byte, 32*1024)
	for i := range payload {
		payload[i] = byte(i)
	}
	for i := 0; i < total; i++ {
		require.NoError(t, ps.Publish("iso_evt", payload))
	}
	require.NoError(t, ps.pubConn.FlushTimeout(testutil.WaitShort))

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if fastCount.Load() >= total && slowDrops.Load() >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	close(release)

	require.GreaterOrEqual(t, fastCount.Load(), int64(total),
		"healthy subscriber must receive all messages despite slow peer")
	require.GreaterOrEqual(t, slowDrops.Load(), int64(1),
		"slow subscriber must receive at least one ErrDroppedMessages")
}

// TestPerSubConn_SubscribeLatency asserts that opening a new per-sub
// connection stays cheap. Threshold bumped under -race.
func TestPerSubConn_SubscribeLatency(t *testing.T) {
	t.Parallel()
	ps := newInternalPubsub(t, Options{})

	const iters = 100
	cancels := make([]func(), 0, iters)
	t.Cleanup(func() {
		for _, c := range cancels {
			c()
		}
	})

	start := time.Now()
	for i := 0; i < iters; i++ {
		c, err := ps.Subscribe(fmt.Sprintf("lat_evt_%d", i), func(context.Context, []byte) {})
		require.NoError(t, err)
		cancels = append(cancels, c)
	}
	mean := time.Since(start) / iters

	bound := 10 * time.Millisecond
	if raceEnabled {
		bound = 50 * time.Millisecond
	}
	require.Lessf(t, mean, bound,
		"mean Subscribe latency %s exceeds bound %s (race=%v)", mean, bound, raceEnabled)
}
