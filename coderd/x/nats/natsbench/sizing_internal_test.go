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

	t.Run("TracksBusiestSubscriberPlusHeadroom", func(t *testing.T) {
		t.Parallel()
		pl := plan{expectPerSub: []int{10000, 90000, 50000}}
		require.Equal(t, 90000+probeHeadroom, derivedLocalQueueMsgs(pl))
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
		pl := buildPlan(Config{
			Messages: 100, Publishers: 1, Subjects: 1, Subscribers: 1, Replicas: 1,
		})
		require.Equal(t, nats.DefaultMaxPending, derivedMaxPending(pl, Payload8KB))
	})

	t.Run("SumsSubjectsSharingANode", func(t *testing.T) {
		t.Parallel()
		// 10 subjects across 5 nodes: each node's lone subscribe conn
		// carries two coalesced subscriptions of 10k messages each.
		pl := buildPlan(Config{
			Messages: 100000, Publishers: 10, Subjects: 10, Subscribers: 50, Replicas: 5,
		})
		want := 2 * int64(10000+probeHeadroom) * int64(Payload64KB+perMessageOverhead)
		require.Equal(t, want, derivedMaxPending(pl, Payload64KB))
		require.Greater(t, want, nats.DefaultMaxPending)
	})

	t.Run("SingleNodeCarriesAllSubjects", func(t *testing.T) {
		t.Parallel()
		pl := buildPlan(Config{
			Messages: 100000, Publishers: 10, Subjects: 10, Subscribers: 50, Replicas: 1,
		})
		want := 10 * int64(10000+probeHeadroom) * int64(Payload64KB+perMessageOverhead)
		require.Equal(t, want, derivedMaxPending(pl, Payload64KB))
	})
}

func TestDerivedQueueBytes(t *testing.T) {
	t.Parallel()

	pl := buildPlan(Config{
		Messages: 100000, Publishers: 10, Subjects: 10, Subscribers: 50, Replicas: 1,
	})
	want := (10000 + probeHeadroom) * (Payload64KB + perMessageOverhead)
	require.Equal(t, want, derivedQueueBytes(pl, Payload64KB))
}

func TestApplySizing(t *testing.T) {
	t.Parallel()

	pl := buildPlan(Config{
		Messages: 10000, Publishers: 1, Subjects: 1, Subscribers: 1, Replicas: 1,
	})

	t.Run("DerivesWhenUnset", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		cfg := applySizing(ctx, slog.Make(), Config{PayloadSize: Payload64KB}, pl)
		require.Equal(t, 10000+probeHeadroom, cfg.LocalQueueMsgs)
		require.Equal(t, derivedQueueBytes(pl, Payload64KB), cfg.LocalQueueBytes)
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
