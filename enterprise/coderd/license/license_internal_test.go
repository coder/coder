package license

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"golang.org/x/xerrors"
)

func TestNextLicenseValidityPeriod(t *testing.T) {
	t.Parallel()

	t.Run("Apply", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name string

			licensePeriods  [][2]time.Time
			expectedPeriods [][2]time.Time
		}{
			{
				name:            "None",
				licensePeriods:  [][2]time.Time{},
				expectedPeriods: [][2]time.Time{},
			},
			{
				name: "One",
				licensePeriods: [][2]time.Time{
					{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
				},
				expectedPeriods: [][2]time.Time{
					{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
				},
			},
			{
				name: "TwoOverlapping",
				licensePeriods: [][2]time.Time{
					{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)},
					{time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC)},
				},
				expectedPeriods: [][2]time.Time{
					{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC)},
				},
			},
			{
				name: "TwoNonOverlapping",
				licensePeriods: [][2]time.Time{
					{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
					{time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC)},
				},
				expectedPeriods: [][2]time.Time{
					{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
					{time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC)},
				},
			},
			{
				name: "ThreeOverlapping",
				licensePeriods: [][2]time.Time{
					{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)},
					{time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC)},
					{time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)},
				},
				expectedPeriods: [][2]time.Time{
					{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)},
				},
			},
			{
				name: "ThreeNonOverlapping",
				licensePeriods: [][2]time.Time{
					{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
					{time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC)},
					{time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)},
				},
				expectedPeriods: [][2]time.Time{
					{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)},
					{time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC)},
					{time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)},
				},
			},
			{
				name: "PeriodContainsAnotherPeriod",
				licensePeriods: [][2]time.Time{
					{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 8, 0, 0, 0, 0, time.UTC)},
					{time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)},
				},
				expectedPeriods: [][2]time.Time{
					{time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 8, 0, 0, 0, 0, time.UTC)},
				},
			},
			{
				name: "EndBeforeStart",
				licensePeriods: [][2]time.Time{
					{time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
				expectedPeriods: nil,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Test with all possible permutations of the periods to ensure
				// consistency regardless of the order.
				ps := permutations(tc.licensePeriods)
				for _, p := range ps {
					t.Logf("permutation: %v", p)
					period := &licenseValidityPeriod{}
					for _, times := range p {
						t.Logf("applying %v", times)
						period.Apply(times[0], times[1])
					}
					assert.Equal(t, tc.expectedPeriods, period.merged(), "merged")
				}
			})
		}
	})
}

func permutations[T any](arr []T) [][]T {
	var res [][]T
	var helper func([]T, int)
	helper = func(a []T, i int) {
		if i == len(a)-1 {
			// make a copy before appending
			tmp := make([]T, len(a))
			copy(tmp, a)
			res = append(res, tmp)
			return
		}
		for j := i; j < len(a); j++ {
			a[i], a[j] = a[j], a[i]
			helper(a, i+1)
			a[i], a[j] = a[j], a[i] // backtrack
		}
	}
	helper(arr, 0)
	return res
}

func TestAgentRuntimeMsToHours(t *testing.T) {
	t.Parallel()

	const hourMs = int64(60 * 60 * 1000)

	testCases := []struct {
		name string
		ms   int64
		want int64
	}{
		{"Zero", 0, 0},
		// Any runtime below an hour floors to zero.
		{"OneMillisecond", 1, 0},
		{"JustUnderAnHour", hourMs - 1, 0},
		{"ExactlyOneHour", hourMs, 1},
		{"JustOverAnHour", hourMs + 1, 1},
		{"JustUnderTwoHours", 2*hourMs - 1, 1},
		{"ExactlyTwoHours", 2 * hourMs, 2},
		// A realistic month of continuous runtime.
		{"Large", 720 * hourMs, 720},
		// Negative input is not expected from the production query, which
		// coalesces NULL to 0, but it must never produce a negative hour
		// count that would compare oddly against the license limits.
		{"Negative", -1, 0},
		{"NegativeHour", -hourMs, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, agentRuntimeMsToHours(tc.ms))
		})
	}
}

func TestAgentRuntimeMsToHoursDivisor(t *testing.T) {
	t.Parallel()

	// Pins the divisor: the maximum int64 of milliseconds converts to the
	// exact whole-hour count only when the divisor is milliseconds per hour.
	assert.EqualValues(t, math.MaxInt64/3_600_000, agentRuntimeMsToHours(math.MaxInt64))
}

// TestUsageMeasurementAborted pins the abort classification both layers of
// the usage-failure policy share: measureUsage's abort decision and the
// measurement closures' log suppression.
func TestUsageMeasurementAborted(t *testing.T) {
	t.Parallel()

	liveCtx := context.Background()
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	// Postgres reports SQLSTATE 57014 for client cancels and for
	// statement_timeout kills alike, so the code alone cannot identify an
	// abort.
	queryCanceled := &pq.Error{Code: "57014", Message: "canceling statement due to user request"}
	statementTimeout := &pq.Error{Code: "57014", Message: "canceling statement due to statement timeout"}

	// Success is never an abort, whatever the context state.
	assert.False(t, usageMeasurementAborted(liveCtx, nil))
	assert.False(t, usageMeasurementAborted(canceledCtx, nil))

	// Failures with a live context degrade into the stable diagnostic,
	// even when Postgres phrases them as cancels: a statement_timeout
	// abort here would wedge every refresh and coderd startup.
	assert.False(t, usageMeasurementAborted(liveCtx, statementTimeout))
	assert.False(t, usageMeasurementAborted(liveCtx, queryCanceled))
	assert.False(t, usageMeasurementAborted(liveCtx, xerrors.New("kaboom")))

	// Any failure while our own context is dead aborts: the caller went
	// away, so nothing should be published or logged.
	assert.True(t, usageMeasurementAborted(canceledCtx, context.Canceled))
	assert.True(t, usageMeasurementAborted(canceledCtx, queryCanceled))
	assert.True(t, usageMeasurementAborted(canceledCtx, xerrors.New("kaboom")))
}
