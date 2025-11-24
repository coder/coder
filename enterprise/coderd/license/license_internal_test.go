package license

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
