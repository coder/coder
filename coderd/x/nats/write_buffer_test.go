package nats //nolint:testpackage // Uses internal pubConns/subConns fields to assert per-conn options.

import (
	"context"
	"testing"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/testutil"
)

// TestWriteBufferSize_AppliedToPools verifies that a positive
// Options.WriteBufferSize propagates to every wrapper-owned client
// connection: every conn in pubConns and every conn in subConns must
// report nc.Opts.WriteBufferSize equal to the option value. Both pools
// are sized > 1 so a per-conn miss is detectable.
func TestWriteBufferSize_AppliedToPools(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	const want = 1 << 20 // 1 MiB
	ps, err := New(ctx, logger, Options{
		PublishConns:    3,
		SubscribeConns:  2,
		WriteBufferSize: want,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })

	require.Len(t, ps.pubConns, 3, "PublishConns must materialize the requested pool size")
	require.Len(t, ps.subConns, 2, "SubscribeConns must materialize the requested pool size")
	for i, nc := range ps.pubConns {
		require.Equal(t, want, nc.Opts.WriteBufferSize,
			"pubConns[%d] write buffer size should equal Options.WriteBufferSize", i)
	}
	for i, nc := range ps.subConns {
		require.Equal(t, want, nc.Opts.WriteBufferSize,
			"subConns[%d] write buffer size should equal Options.WriteBufferSize", i)
	}
}

// TestWriteBufferSize_ZeroPreservesNATSDefault verifies that omitting
// Options.WriteBufferSize leaves nc.Opts.WriteBufferSize at the
// upstream nats.go default (32 KiB), so existing callers see no
// behavior change.
func TestWriteBufferSize_ZeroPreservesNATSDefault(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	ps, err := New(ctx, logger, Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ps.Close() })

	// natsgo.DefaultWriteBufSize is the documented zero-default the
	// client falls back to when no WriteBufferSize option is supplied.
	want := natsgo.DefaultWriteBufSize
	require.Greater(t, want, 0, "nats.go must expose a positive default write buffer size")
	for i, nc := range ps.pubConns {
		require.Equal(t, want, nc.Opts.WriteBufferSize,
			"pubConns[%d] should keep nats.go default when WriteBufferSize is zero", i)
	}
	for i, nc := range ps.subConns {
		require.Equal(t, want, nc.Opts.WriteBufferSize,
			"subConns[%d] should keep nats.go default when WriteBufferSize is zero", i)
	}
}

// TestWriteBufferSize_NewFromConnIgnored verifies that NewFromConn
// neither rejects nor mutates an externally-supplied connection's
// write buffer: the caller's *natsgo.Conn must be reused as-is and
// its Opts.WriteBufferSize must match whatever the caller dialed with.
// This documents the divergence captured in Options.WriteBufferSize.
func TestWriteBufferSize_NewFromConnIgnored(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	// host opens an in-process pool we can borrow a conn from. We pin
	// its write buffer to a non-default value so we can prove
	// NewFromConn leaves it alone.
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()
	const external = 2 << 20 // 2 MiB
	host, err := New(ctx, logger, Options{
		PublishConns:    1,
		SubscribeConns:  1,
		WriteBufferSize: external,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = host.Close() })

	externalConn := host.pubConns[0]
	require.Equal(t, external, externalConn.Opts.WriteBufferSize,
		"sanity: host conn should already have the configured write buffer size")

	p, err := NewFromConn(logger, externalConn)
	require.NoError(t, err)
	require.Len(t, p.pubConns, 1)
	require.Len(t, p.subConns, 1)
	require.Same(t, externalConn, p.pubConns[0], "NewFromConn must alias the supplied conn for publishes")
	require.Same(t, externalConn, p.subConns[0], "NewFromConn must alias the supplied conn for subscribes")
	// The supplied conn's write buffer must be exactly what the caller
	// dialed with; NewFromConn does not own or reconfigure it.
	require.Equal(t, external, p.pubConns[0].Opts.WriteBufferSize,
		"NewFromConn must not alter the external conn's write buffer")
	require.NoError(t, p.Close())
	require.False(t, externalConn.IsClosed(),
		"Close on a NewFromConn Pubsub must not close the external conn")
}
