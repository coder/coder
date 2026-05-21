//nolint:testpackage // Uses internal fields and helpers for pool assertions.
package nats

import (
	"context"
	"fmt"
	"testing"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

func newPoolPubsub(t *testing.T, publishConns int) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{PublishConns: publishConns})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })
	return ps
}

func TestPublishPool_CreatesN(t *testing.T) {
	t.Parallel()
	const n = 4
	ps := newPoolPubsub(t, n)

	require.Len(t, ps.publishPool, n)
	require.Len(t, ps.subscribePool, 1)
	seen := make(map[*natsgo.Conn]struct{}, n)
	for i, nc := range ps.publishPool {
		require.NotNil(t, nc, "publishPool[%d] must be non-nil", i)
		require.True(t, nc.IsConnected(), "publishPool[%d] must be connected", i)
		require.NotSame(t, nc, ps.subscribePool[0], "publishPool[%d] must be distinct from subscribePool[0]", i)
		_, dup := seen[nc]
		require.False(t, dup, "publishPool[%d] must be distinct", i)
		seen[nc] = struct{}{}
	}
	require.Equal(t, n+1, ps.ns.NumClients())
}

func TestPublishPool_PickPubConn_DistributesSubjects(t *testing.T) {
	t.Parallel()
	const n = 4
	ps := newPoolPubsub(t, n)

	counts := make(map[*natsgo.Conn]int, n)
	for i := 0; i < 64; i++ {
		subj := fmt.Sprintf("legacy.event_%03d", i)
		counts[pickConn(ps.publishPool, subj)]++
	}
	require.GreaterOrEqual(t, len(counts), 2)
}

func TestPublishPool_CloseClosesAllOwnedConns(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	const n = 3
	ps, err := New(ctx, logger, Options{PublishConns: n})
	require.NoError(t, err)
	require.Len(t, ps.publishPool, n)

	conns := append([]*natsgo.Conn(nil), ps.publishPool...)
	subscribePool := append([]*natsgo.Conn(nil), ps.subscribePool...)

	require.NoError(t, ps.Close())
	for i, nc := range conns {
		require.True(t, nc.IsClosed(), "publishPool[%d] must be closed after Close", i)
	}
	for i, nc := range subscribePool {
		require.True(t, nc.IsClosed(), "subscribePool[%d] must be closed after Close", i)
	}
	require.NoError(t, ps.Close())
}
