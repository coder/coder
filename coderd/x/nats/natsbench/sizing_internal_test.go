package natsbench

import (
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/nats"
	"github.com/coder/coder/v2/testutil"
)

func TestDerivedLocalQueueMsgs(t *testing.T) {
	t.Parallel()

	t.Run("FloorsAtDefault", func(t *testing.T) {
		t.Parallel()
		pl := plan{expectPerSub: []int{1, 2, 3}}
		require.Equal(t, minLocalQueueMsgs, derivedLocalQueueMsgs(pl))
	})

	t.Run("TracksBusiestSubscriber", func(t *testing.T) {
		t.Parallel()
		pl := plan{expectPerSub: []int{10_000, 90_000, 50_000}}
		require.Equal(t, 90_000, derivedLocalQueueMsgs(pl))
	})

	t.Run("Caps", func(t *testing.T) {
		t.Parallel()
		pl := plan{expectPerSub: []int{maxLocalQueueMsgs + 1}}
		require.Equal(t, maxLocalQueueMsgs, derivedLocalQueueMsgs(pl))
	})
}

func TestDerivedMaxPending(t *testing.T) {
	t.Parallel()

	t.Run("FloorsAtPackageDefault", func(t *testing.T) {
		t.Parallel()
		pl := plan{expectPerSub: []int{100}}
		require.Equal(t, nats.DefaultMaxPending, derivedMaxPending(pl, Payload8KB))
	})

	t.Run("GrowsWithBurst", func(t *testing.T) {
		t.Parallel()
		pl := plan{expectPerSub: []int{10_000}}
		want := int64(10_000) * int64(Payload64KB+perMessageOverhead)
		require.Equal(t, want, derivedMaxPending(pl, Payload64KB))
		require.Greater(t, want, nats.DefaultMaxPending)
	})
}

func TestApplySizing(t *testing.T) {
	t.Parallel()

	pl := plan{expectPerSub: []int{10_000}}

	t.Run("DerivesWhenUnset", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		cfg := applySizing(ctx, slog.Make(), Config{PayloadSize: Payload64KB}, pl)
		require.Equal(t, 10_000, cfg.LocalQueueMsgs)
		require.Equal(t, derivedMaxPending(pl, Payload64KB), cfg.MaxPending)
	})

	t.Run("HonorsExplicitValues", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		cfg := applySizing(ctx, slog.Make(), Config{
			PayloadSize:    Payload64KB,
			LocalQueueMsgs: 64,
			MaxPending:     1,
		}, pl)
		require.Equal(t, 64, cfg.LocalQueueMsgs)
		require.EqualValues(t, 1, cfg.MaxPending)
	})
}
