package usage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNextBoundary(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name     string
		T        time.Time
		interval time.Duration
		expected time.Time
	}{
		{
			name:     "exactly_on_boundary",
			T:        time.Date(2023, 1, 1, 8, 0, 0, 0, time.UTC),
			interval: 4 * time.Hour,
			// On a boundary → returns the next one.
			expected: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "1ns_after_boundary",
			T:        time.Date(2023, 1, 1, 8, 0, 0, 1, time.UTC),
			interval: 4 * time.Hour,
			expected: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "1ns_before_boundary",
			T:        time.Date(2023, 1, 1, 7, 59, 59, 999999999, time.UTC),
			interval: 4 * time.Hour,
			expected: time.Date(2023, 1, 1, 8, 0, 0, 0, time.UTC),
		},
		{
			name:     "mid_interval",
			T:        time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC),
			interval: 4 * time.Hour,
			expected: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			name:     "5min_interval",
			T:        time.Date(2026, 3, 13, 14, 2, 30, 0, time.UTC),
			interval: 5 * time.Minute,
			expected: time.Date(2026, 3, 13, 14, 5, 0, 0, time.UTC),
		},
		{
			name:     "1hr_interval",
			T:        time.Date(2026, 6, 15, 9, 45, 0, 0, time.UTC),
			interval: 1 * time.Hour,
			expected: time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := nextBoundary(tc.T, tc.interval)
			require.Equal(t, tc.expected, got)
		})
	}
}

func TestNextTick(t *testing.T) {
	t.Parallel()

	t.Run("NoJitter", func(t *testing.T) {
		t.Parallel()

		now := time.Date(2026, 3, 13, 14, 2, 30, 0, time.UTC)
		interval := 4 * time.Hour

		boundary, delay := nextTick(now, interval, 0)

		expectedBoundary := time.Date(2026, 3, 13, 16, 0, 0, 0, time.UTC)
		require.Equal(t, expectedBoundary, boundary)
		require.Equal(t, boundary.Sub(now), delay)
	})

	t.Run("WithJitter", func(t *testing.T) {
		t.Parallel()

		now := time.Date(2026, 3, 13, 14, 2, 30, 0, time.UTC)
		interval := 4 * time.Hour
		jitter := 10 * time.Minute

		boundary, delay := nextTick(now, interval, jitter)

		expectedBoundary := time.Date(2026, 3, 13, 16, 0, 0, 0, time.UTC)
		require.Equal(t, expectedBoundary, boundary)

		base := boundary.Sub(now)
		require.GreaterOrEqual(t, delay, base,
			"delay must be at least the base distance to boundary")
		require.Less(t, delay, base+jitter,
			"delay must be less than base + jitter")
	})
}
