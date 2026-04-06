package chatd //nolint:testpackage // Keeps chatd unit tests in the package.

import (
	"testing"
	"time"

	"github.com/coder/coder/v2/codersdk"
)

func TestComputeUsagePeriodBounds(t *testing.T) {
	t.Parallel()

	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load America/New_York: %v", err)
	}

	tests := []struct {
		name      string
		now       time.Time
		period    codersdk.ChatUsageLimitPeriod
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			name:      "day/mid_day",
			now:       time.Date(2025, time.June, 15, 14, 30, 0, 0, time.UTC),
			period:    codersdk.ChatUsageLimitPeriodDay,
			wantStart: time.Date(2025, time.June, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.June, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "day/midnight_exactly",
			now:       time.Date(2025, time.June, 15, 0, 0, 0, 0, time.UTC),
			period:    codersdk.ChatUsageLimitPeriodDay,
			wantStart: time.Date(2025, time.June, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.June, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "day/end_of_day",
			now:       time.Date(2025, time.June, 15, 23, 59, 59, 0, time.UTC),
			period:    codersdk.ChatUsageLimitPeriodDay,
			wantStart: time.Date(2025, time.June, 15, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.June, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "week/wednesday",
			now:       time.Date(2025, time.June, 11, 10, 0, 0, 0, time.UTC),
			period:    codersdk.ChatUsageLimitPeriodWeek,
			wantStart: time.Date(2025, time.June, 9, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.June, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "week/monday",
			now:       time.Date(2025, time.June, 9, 0, 0, 0, 0, time.UTC),
			period:    codersdk.ChatUsageLimitPeriodWeek,
			wantStart: time.Date(2025, time.June, 9, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.June, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "week/sunday",
			now:       time.Date(2025, time.June, 15, 23, 0, 0, 0, time.UTC),
			period:    codersdk.ChatUsageLimitPeriodWeek,
			wantStart: time.Date(2025, time.June, 9, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.June, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "week/year_boundary",
			now:       time.Date(2024, time.December, 31, 12, 0, 0, 0, time.UTC),
			period:    codersdk.ChatUsageLimitPeriodWeek,
			wantStart: time.Date(2024, time.December, 30, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.January, 6, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "month/mid_month",
			now:       time.Date(2025, time.June, 15, 0, 0, 0, 0, time.UTC),
			period:    codersdk.ChatUsageLimitPeriodMonth,
			wantStart: time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.July, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "month/first_day",
			now:       time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC),
			period:    codersdk.ChatUsageLimitPeriodMonth,
			wantStart: time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.July, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "month/last_day",
			now:       time.Date(2025, time.June, 30, 23, 59, 59, 0, time.UTC),
			period:    codersdk.ChatUsageLimitPeriodMonth,
			wantStart: time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.July, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "month/february",
			now:       time.Date(2025, time.February, 15, 12, 0, 0, 0, time.UTC),
			period:    codersdk.ChatUsageLimitPeriodMonth,
			wantStart: time.Date(2025, time.February, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.March, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "month/leap_year_february",
			now:       time.Date(2024, time.February, 29, 12, 0, 0, 0, time.UTC),
			period:    codersdk.ChatUsageLimitPeriodMonth,
			wantStart: time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:      "day/non_utc_timezone",
			now:       time.Date(2025, time.June, 15, 22, 0, 0, 0, newYork),
			period:    codersdk.ChatUsageLimitPeriodDay,
			wantStart: time.Date(2025, time.June, 16, 0, 0, 0, 0, time.UTC),
			wantEnd:   time.Date(2025, time.June, 17, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			start, end := ComputeUsagePeriodBounds(tc.now, tc.period)
			if !start.Equal(tc.wantStart) {
				t.Errorf("start: got %v, want %v", start, tc.wantStart)
			}
			if !end.Equal(tc.wantEnd) {
				t.Errorf("end: got %v, want %v", end, tc.wantEnd)
			}
		})
	}
}
