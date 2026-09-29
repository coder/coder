package chatstate_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/testutil"
)

// TestUpdateRecordsTransitionMetrics verifies that Update records one
// observation per transaction in the registered histograms, labeled
// with the transitions the callback ran and whether it committed.
func TestUpdateRecordsTransitionMetrics(t *testing.T) {
	t.Parallel()
	f := newTestFixture(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	created := createTestChat(t, f)

	reg := prometheus.NewRegistry()
	metrics := chatstate.NewMetrics(reg)
	m := chatstate.NewChatMachine(f.DB, f.Pub, created.Chat.ID).WithMetrics(metrics)

	require.NoError(t, m.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.FinishTurn(chatstate.FinishTurnInput{})
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

	counts := histogramCounts(t, reg, "coderd_chatd_transition_duration_seconds")
	require.Equal(t, uint64(1), counts["FinishTurn|committed"])
	require.Equal(t, uint64(1), counts["SetArchived|rolled_back"])
	require.Equal(t, uint64(1), counts["none|committed"])
	lockWaits := histogramCounts(t, reg, "coderd_chatd_transition_lock_wait_seconds")
	require.Equal(t, uint64(1), lockWaits["FinishTurn|committed"])
}

// histogramCounts returns the sample count of every series of the named
// histogram keyed by "transition|outcome".
func histogramCounts(t *testing.T, reg *prometheus.Registry, name string) map[string]uint64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	counts := map[string]uint64{}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			var transition, outcome string
			for _, label := range metric.GetLabel() {
				switch label.GetName() {
				case "transition":
					transition = label.GetValue()
				case "outcome":
					outcome = label.GetValue()
				}
			}
			counts[transition+"|"+outcome] = metric.GetHistogram().GetSampleCount()
		}
	}
	require.NotEmpty(t, counts, "histogram %s must be registered", name)
	return counts
}
