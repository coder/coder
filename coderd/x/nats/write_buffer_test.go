package nats //nolint:testpackage // Uses internal publishPool/subscribePool fields to assert per-conn options.

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
// connection: every conn in publishPool and every conn in subscribePool must
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

	require.Len(t, ps.publishPool, 3, "PublishConns must materialize the requested pool size")
	require.Len(t, ps.subscribePool, 2, "SubscribeConns must materialize the requested pool size")
	for i, nc := range ps.publishPool {
		require.Equal(t, want, nc.Opts.WriteBufferSize,
			"publishPool[%d] write buffer size should equal Options.WriteBufferSize", i)
	}
	for i, nc := range ps.subscribePool {
		require.Equal(t, want, nc.Opts.WriteBufferSize,
			"subscribePool[%d] write buffer size should equal Options.WriteBufferSize", i)
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
	for i, nc := range ps.publishPool {
		require.Equal(t, want, nc.Opts.WriteBufferSize,
			"publishPool[%d] should keep nats.go default when WriteBufferSize is zero", i)
	}
	for i, nc := range ps.subscribePool {
		require.Equal(t, want, nc.Opts.WriteBufferSize,
			"subscribePool[%d] should keep nats.go default when WriteBufferSize is zero", i)
	}
}


