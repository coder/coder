//nolint:testpackage
package nats

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

func TestServerStatsReportsEmbeddedServerCounters(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
		Named("serverstats").Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	p, err := New(ctx, logger, Options{
		ServerName:   "serverstats-test",
		ReadyTimeout: testutil.WaitMedium,
		// Explicit MaxPending so the test asserts on a known value
		// rather than the package default.
		MaxPending: 256 << 20,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })

	stats, ok := p.ServerStats()
	require.True(t, ok, "ServerStats should report ok for a Pubsub with an embedded server")
	// At least the wrapper's own publish + subscribe conns are clients.
	require.GreaterOrEqual(t, stats.NumClients, 2, "expected wrapper-owned client conns")
	// No peers, so no routes should be established.
	require.Equal(t, 0, stats.NumRoutes)
	// No load yet: slow-consumer and stale counters should be zero.
	require.Zero(t, stats.NumSlowConsumers)
	require.Zero(t, stats.NumSlowConsumersClients)
	require.Zero(t, stats.NumSlowConsumersRoutes)
	require.Zero(t, stats.NumStaleConnections)
	require.Equal(t, int64(256<<20), stats.MaxPending,
		"MaxPending should mirror the option")
}

func TestServerStatsDefaultMaxPendingWhenZero(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).
		Named("serverstats-default").Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitMedium)
	defer cancel()
	p, err := New(ctx, logger, Options{
		ServerName:   "serverstats-default-test",
		ReadyTimeout: testutil.WaitMedium,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = p.Close() })

	stats, ok := p.ServerStats()
	require.True(t, ok)
	require.Equal(t, DefaultMaxPending, stats.MaxPending)
}

func TestServerStatsReturnsFalseWithoutEmbeddedServer(t *testing.T) {
	t.Parallel()
	// A nil-Pubsub call must not panic and must report not-ok.
	stats, ok := (*Pubsub)(nil).ServerStats()
	require.False(t, ok)
	require.Equal(t, ServerStats{}, stats)
}
