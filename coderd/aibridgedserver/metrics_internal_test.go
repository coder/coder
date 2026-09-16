package aibridgedserver

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestUpdateBlockedUsers(t *testing.T) {
	t.Parallel()

	groupA := uuid.New()
	groupB := uuid.New()

	// Each round is one updateBlockedUsers call and the rows its query returns.
	// The final gauge state is asserted after the last round, so sequential
	// rounds cover Reset clearing series that dropped to zero.
	type round struct {
		rows []database.GetOverBudgetUsersPerGroupRow
		err  error
	}
	tests := []struct {
		name       string
		rounds     []round
		wantErr    error
		wantCount  int
		wantValues map[uuid.UUID]float64
	}{
		{
			name: "SetsPerGroupGauges",
			rounds: []round{{rows: []database.GetOverBudgetUsersPerGroupRow{
				{GroupID: groupA, OverBudgetUsers: 3},
				{GroupID: groupB, OverBudgetUsers: 1},
			}}},
			wantCount:  2,
			wantValues: map[uuid.UUID]float64{groupA: 3, groupB: 1},
		},
		{
			// groupB drops to zero in the second round, so its stale series
			// is cleared by Reset.
			name: "ResetClearsStaleSeries",
			rounds: []round{
				{rows: []database.GetOverBudgetUsersPerGroupRow{
					{GroupID: groupA, OverBudgetUsers: 3},
					{GroupID: groupB, OverBudgetUsers: 1},
				}},
				{rows: []database.GetOverBudgetUsersPerGroupRow{
					{GroupID: groupA, OverBudgetUsers: 2},
				}},
			},
			wantCount:  1,
			wantValues: map[uuid.UUID]float64{groupA: 2},
		},
		{
			name:    "PropagatesDBError",
			rounds:  []round{{err: sql.ErrConnDone}},
			wantErr: sql.ErrConnDone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Given: a database returning the over-budget rows for each round.
			db := dbmock.NewMockStore(gomock.NewController(t))
			calls := make([]any, 0, len(tt.rounds))
			for _, r := range tt.rounds {
				calls = append(calls, db.EXPECT().GetOverBudgetUsersPerGroup(gomock.Any(), gomock.Any()).
					Return(r.rows, r.err))
			}
			gomock.InOrder(calls...)

			m := NewMetrics(prometheus.NewRegistry())
			clk := quartz.NewMock(t)

			// When: the gauge is updated once per round.
			var err error
			for range tt.rounds {
				err = m.updateBlockedUsers(t.Context(), clk, db, codersdk.AIBudgetPeriodMonth)
			}

			// Then: the query error propagates, or the gauge holds the final
			// per-group counts.
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantCount, promtest.CollectAndCount(m.BlockedUsers))
			for group, want := range tt.wantValues {
				require.Equal(t, want, promtest.ToFloat64(m.BlockedUsers.WithLabelValues(group.String())))
			}
		})
	}
}

func TestStartBlockedUsersCollector(t *testing.T) {
	t.Parallel()

	t.Run("NilReceiverNoop", func(t *testing.T) {
		t.Parallel()

		// Given: a nil Metrics.
		var m *Metrics

		// When: the collector is started.
		closeFn := m.StartBlockedUsersCollector(t.Context(), testutil.Logger(t), quartz.NewMock(t), nil, codersdk.AIBudgetPeriodMonth, time.Minute)

		// Then: the returned closer is a no-op and does not panic.
		require.NotPanics(t, closeFn)
	})

	t.Run("TicksAndStops", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		groupA := uuid.New()

		// Given: a running collector on a mock clock.
		db := dbmock.NewMockStore(gomock.NewController(t))
		m := NewMetrics(prometheus.NewRegistry())
		clk := quartz.NewMock(t)
		db.EXPECT().GetOverBudgetUsersPerGroup(gomock.Any(), gomock.Any()).
			Return([]database.GetOverBudgetUsersPerGroupRow{
				{GroupID: groupA, OverBudgetUsers: 4},
			}, nil).AnyTimes()

		closeFn := m.StartBlockedUsersCollector(ctx, testutil.Logger(t), clk, db, codersdk.AIBudgetPeriodMonth, time.Minute)
		defer closeFn()

		// When: the ticker fires.
		_, w := clk.AdvanceNext()
		w.MustWait(ctx)

		// Then: the gauge reflects the queried per-group count.
		require.Eventually(t, func() bool {
			return promtest.ToFloat64(m.BlockedUsers.WithLabelValues(groupA.String())) == 4.0
		}, testutil.WaitShort, testutil.IntervalFast)
	})
}
