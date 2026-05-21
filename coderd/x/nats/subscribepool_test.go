//nolint:testpackage // Uses internal fields and helpers for sub-pool assertions.
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

func newSubPoolPubsub(t *testing.T, subscribePool int) *Pubsub {
	t.Helper()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{SubscribeConns: subscribePool})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })
	return ps
}

func TestSubscribePool_CreatesN(t *testing.T) {
	t.Parallel()
	const n = 4
	ps := newSubPoolPubsub(t, n)

	require.Len(t, ps.subscribePool, n)
	seen := make(map[*natsgo.Conn]struct{}, n)
	for i, nc := range ps.subscribePool {
		require.NotNil(t, nc, "subscribePool[%d] must be non-nil", i)
		require.True(t, nc.IsConnected(), "subscribePool[%d] must be connected", i)
		require.NotSame(t, nc, ps.publishPool[0], "subscribePool[%d] must be distinct from publishPool[0]", i)
		_, dup := seen[nc]
		require.False(t, dup, "subscribePool[%d] must be distinct", i)
		seen[nc] = struct{}{}
	}
	require.Equal(t, n+1, ps.ns.NumClients())
}

func TestSubscribePool_SharedSubsDistributedAcrossConns(t *testing.T) {
	t.Parallel()
	const n = 4
	ps := newSubPoolPubsub(t, n)

	const total = 64
	cancels := make([]func(), 0, total)
	expected := make(map[*natsgo.Conn]int, n)
	for i := 0; i < total; i++ {
		evt := fmt.Sprintf("distrib_evt_%04d", i)
		expected[pickConn(ps.subscribePool, evt)]++
		c, err := ps.Subscribe(evt, func(context.Context, []byte) {})
		require.NoError(t, err)
		cancels = append(cancels, c)
	}
	t.Cleanup(func() {
		for _, c := range cancels {
			c()
		}
	})

	require.GreaterOrEqual(t, len(expected), 2)
	for _, nc := range ps.subscribePool {
		require.Equal(t, expected[nc], nc.NumSubscriptions())
	}
}
