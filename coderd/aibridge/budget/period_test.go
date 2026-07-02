package budget_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge/budget"
	"github.com/coder/coder/v2/codersdk"
)

func TestCurrentPeriod(t *testing.T) {
	t.Parallel()

	nonUTC := time.FixedZone("UTC-4", -4*60*60)

	tests := []struct {
		name      string
		now       time.Time
		period    codersdk.AIBudgetPeriod
		wantStart time.Time
		wantEnd   time.Time
		wantErr   string
	}{
		{
			name:      "MidMonthUTC",
			now:       time.Date(2026, time.March, 15, 12, 30, 45, 0, time.UTC),
			period:    codersdk.AIBudgetPeriodMonth,
			wantStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "FirstInstantOfMonthUTC",
			now:       time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
			period:    codersdk.AIBudgetPeriodMonth,
			wantStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "LastInstantOfMonthUTC",
			now:       time.Date(2026, time.March, 31, 23, 59, 59, 999_999_999, time.UTC),
			period:    codersdk.AIBudgetPeriodMonth,
			wantStart: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "DecemberRollsToJanuary",
			now:       time.Date(2026, time.December, 15, 12, 0, 0, 0, time.UTC),
			period:    codersdk.AIBudgetPeriodMonth,
			wantStart: time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "NonUTCNormalizedAcrossMonth",
			// Non-UTC input must be normalized before computing the window:
			// 2026-03-31 23:00 at UTC-4 is 2026-04-01 03:00 UTC.
			now:       time.Date(2026, time.March, 31, 23, 0, 0, 0, nonUTC),
			period:    codersdk.AIBudgetPeriodMonth,
			wantStart: time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "UnsupportedPeriod",
			now:     time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC),
			period:  codersdk.AIBudgetPeriod("unknown"),
			wantErr: "unsupported AI budget period",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := budget.CurrentPeriod(tt.now, tt.period)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantStart, got.Start, "start")
			require.Equal(t, tt.wantEnd, got.End, "end")
		})
	}
}
