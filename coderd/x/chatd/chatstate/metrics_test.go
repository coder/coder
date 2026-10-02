package chatstate_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// TestUpdateRecordsTransitionMetrics verifies that Update records one
// observation per transaction, labeled with every transition the
// callback ran (including the ones that validate outside
// requireFromAllowed) and whether it committed, and one observation per
// phase.
func TestUpdateRecordsTransitionMetrics(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	reg := prometheus.NewRegistry()
	metrics := chatstate.NewMetrics(reg)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID).WithMetrics(metrics)

	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.Acquire(chatstate.AcquireInput{WorkerID: uuid.New(), RunnerID: uuid.New()})
		return err
	}))
	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		if _, err := tx.FinishTurn(chatstate.FinishTurnInput{}); err != nil {
			return err
		}
		_, err := tx.Abandon(chatstate.AbandonInput{})
		return err
	}))
	rollback := xerrors.New("abort")
	require.ErrorIs(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		if _, err := tx.SetArchived(chatstate.SetArchivedInput{Archived: true}); err != nil {
			return err
		}
		return rollback
	}), rollback)
	require.NoError(t, m.Update(ctx, func(_ *chatstate.Tx, _ database.Store) error {
		return nil
	}))

	counts := histogramCounts(t, reg, "coderd_chatd_transition_duration_seconds", nil)
	require.Equal(t, uint64(1), counts["Acquire|committed"])
	require.Equal(t, uint64(1), counts["FinishTurn+Abandon|committed"])
	require.Equal(t, uint64(1), counts["SetArchived|rolled_back"])
	require.Equal(t, uint64(1), counts["none|committed"])

	for _, phase := range []string{"pre_lock", "lock_wait", "callback", "commit"} {
		phases := histogramCounts(t, reg, "coderd_chatd_transition_phase_seconds", map[string]string{"phase": phase})
		require.Equal(t, uint64(1), phases["Acquire|committed"], phase)
		require.Equal(t, uint64(1), phases["SetArchived|rolled_back"], phase)
	}
}

// histogramCounts returns the sample count of every series of the named
// histogram whose labels match filter, keyed by "transition|outcome".
func histogramCounts(t *testing.T, reg *prometheus.Registry, name string, filter map[string]string) map[string]uint64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	counts := map[string]uint64{}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			matches := true
			for k, v := range filter {
				if labels[k] != v {
					matches = false
				}
			}
			if !matches {
				continue
			}
			counts[labels["transition"]+"|"+labels["outcome"]] = metric.GetHistogram().GetSampleCount()
		}
	}
	require.NotEmpty(t, counts, "histogram %s must be registered", name)
	return counts
}
