package license

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/codersdk"
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
		// Pins the divisor as milliseconds per hour.
		{"MaxInt64", math.MaxInt64, math.MaxInt64 / hourMs},
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

// TestAppendAgentRuntimeHoursWarning pins the warning arithmetic: thresholds
// are "reached" (>=), and reaching the allocation supersedes the advisory
// soft limit so at most one warning is appended.
func TestAppendAgentRuntimeHoursWarning(t *testing.T) {
	t.Parallel()

	softLimit := new(int64(80))
	softWarning := func(actual int64) []string {
		return []string{fmt.Sprintf(codersdk.LicenseAgentRuntimeHoursSoftLimitWarningText, actual, 100, 80)}
	}
	allocationWarning := func(actual int64) []string {
		return []string{fmt.Sprintf(codersdk.LicenseAgentRuntimeHoursAllocationReachedWarningText, actual, 100)}
	}

	testCases := []struct {
		name       string
		actual     int64
		allocation int64
		softLimit  *int64
		want       []string
	}{
		{"ZeroAllocation", 50, 0, softLimit, nil},
		{"NegativeAllocation", 50, -1, softLimit, nil},
		{"BelowSoftLimit", 79, 100, softLimit, nil},
		{"AtSoftLimit", 80, 100, softLimit, softWarning(80)},
		{"BetweenSoftLimitAndAllocation", 99, 100, softLimit, softWarning(99)},
		{"AtAllocationSupersedesSoftLimit", 100, 100, softLimit, allocationWarning(100)},
		{"OverAllocation", 150, 100, softLimit, allocationWarning(150)},
		{"NoSoftLimitBelowAllocation", 99, 100, nil, nil},
		{"NoSoftLimitAtAllocation", 100, 100, nil, allocationWarning(100)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, appendAgentRuntimeHoursWarning(nil, tc.actual, tc.allocation, tc.softLimit))
		})
	}
}
