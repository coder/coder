// schedule_validation_test.go

package cron_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/schedule/cron"
)

func TestParseRange(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		input     string
		maxValue  int
		expected  map[int]bool
		expectErr bool
	}{
		{
			name:     "Wildcard",
			input:    "*",
			maxValue: 5,
			expected: map[int]bool{
				0: true, 1: true, 2: true, 3: true, 4: true, 5: true,
			},
		},
		{
			name:     "Single value",
			input:    "3",
			maxValue: 5,
			expected: map[int]bool{
				3: true,
			},
		},
		{
			name:     "Range",
			input:    "1-3",
			maxValue: 5,
			expected: map[int]bool{
				1: true, 2: true, 3: true,
			},
		},
		{
			name:     "Complex range",
			input:    "1-3,5,7-9",
			maxValue: 9,
			expected: map[int]bool{
				1: true, 2: true, 3: true, 5: true, 7: true, 8: true, 9: true,
			},
		},
		{
			name:      "Value too high",
			input:     "6",
			maxValue:  5,
			expectErr: true,
		},
		{
			name:      "Range too high",
			input:     "4-6",
			maxValue:  5,
			expectErr: true,
		},
		{
			name:      "Invalid range",
			input:     "3-1",
			maxValue:  5,
			expectErr: true,
		},
		{
			name:      "Invalid value",
			input:     "abc",
			maxValue:  5,
			expectErr: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			result, err := cron.ParseRange(testCase.input, testCase.maxValue)
			if testCase.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.expected, result)
		})
	}
}

func TestCheckOverlap(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		range1    string
		range2    string
		maxValue  int
		overlap   bool
		expectErr bool
	}{
		{
			name:     "Same range",
			range1:   "1-5",
			range2:   "1-5",
			maxValue: 10,
			overlap:  true,
		},
		{
			name:     "Different ranges",
			range1:   "1-3",
			range2:   "4-6",
			maxValue: 10,
			overlap:  false,
		},
		{
			name:     "Overlapping ranges",
			range1:   "1-5",
			range2:   "4-8",
			maxValue: 10,
			overlap:  true,
		},
		{
			name:     "Wildcard overlap",
			range1:   "*",
			range2:   "3-5",
			maxValue: 10,
			overlap:  true,
		},
		{
			name:     "Complex ranges",
			range1:   "1-3,5,7-9",
			range2:   "2-4,6,8-10",
			maxValue: 10,
			overlap:  true,
		},
		{
			name:     "Single values",
			range1:   "1",
			range2:   "1",
			maxValue: 10,
			overlap:  true,
		},
		{
			name:     "Single value vs range",
			range1:   "1",
			range2:   "1-3",
			maxValue: 10,
			overlap:  true,
		},
		{
			name:      "Invalid range - value too high",
			range1:    "11",
			range2:    "1-3",
			maxValue:  10,
			expectErr: true,
		},
		{
			name:      "Invalid range - negative value",
			range1:    "-1",
			range2:    "1-3",
			maxValue:  10,
			expectErr: true,
		},
		{
			name:      "Invalid range - malformed",
			range1:    "1-",
			range2:    "1-3",
			maxValue:  10,
			expectErr: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			overlap, err := cron.CheckOverlap(testCase.range1, testCase.range2, testCase.maxValue)
			if testCase.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.overlap, overlap)
		})
	}
}

func TestOverlapWrappers(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		range1      string
		range2      string
		overlap     bool
		expectErr   bool
		overlapFunc func(string, string) (bool, error)
	}{
		// HoursOverlap tests (max 23)
		{
			name:        "Valid hour range",
			range1:      "23",
			range2:      "23",
			overlap:     true,
			overlapFunc: cron.HoursOverlap,
		},
		{
			name:        "Invalid hour range",
			range1:      "24",
			range2:      "24",
			expectErr:   true,
			overlapFunc: cron.HoursOverlap,
		},

		// MonthsOverlap tests (max 12)
		{
			name:        "Valid month range",
			range1:      "12",
			range2:      "12",
			overlap:     true,
			overlapFunc: cron.MonthsOverlap,
		},
		{
			name:        "Invalid month range",
			range1:      "13",
			range2:      "13",
			expectErr:   true,
			overlapFunc: cron.MonthsOverlap,
		},

		// DomOverlap tests (max 31)
		{
			name:        "Valid day of month range",
			range1:      "31",
			range2:      "31",
			overlap:     true,
			overlapFunc: cron.DomOverlap,
		},
		{
			name:        "Invalid day of month range",
			range1:      "32",
			range2:      "32",
			expectErr:   true,
			overlapFunc: cron.DomOverlap,
		},

		// DowOverlap tests (max 6)
		{
			name:        "Valid day of week range",
			range1:      "6",
			range2:      "6",
			overlap:     true,
			overlapFunc: cron.DowOverlap,
		},
		{
			name:        "Invalid day of week range",
			range1:      "7",
			range2:      "7",
			expectErr:   true,
			overlapFunc: cron.DowOverlap,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			overlap, err := testCase.overlapFunc(testCase.range1, testCase.range2)
			if testCase.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.overlap, overlap)
		})
	}
}

func TestDaysOverlap(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		dom1      string
		dow1      string
		dom2      string
		dow2      string
		overlap   bool
		expectErr bool
	}{
		{
			name:    "DOM overlap only",
			dom1:    "1-15",
			dow1:    "1-3",
			dom2:    "10-20",
			dow2:    "4-6",
			overlap: true, // true because DOM overlaps (10-15)
		},
		{
			name:    "DOW overlap only",
			dom1:    "1-15",
			dow1:    "1-3",
			dom2:    "16-31",
			dow2:    "3-5",
			overlap: true, // true because DOW overlaps (3)
		},
		{
			name:    "Both DOM and DOW overlap",
			dom1:    "1-15",
			dow1:    "1-3",
			dom2:    "10-20",
			dow2:    "3-5",
			overlap: true, // true because both overlap
		},
		{
			name:    "No overlap",
			dom1:    "1-15",
			dow1:    "1-3",
			dom2:    "16-31",
			dow2:    "4-6",
			overlap: false, // false because neither overlaps
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			overlap, err := cron.DaysOverlap(testCase.dom1, testCase.dow1, testCase.dom2, testCase.dow2)
			if testCase.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.overlap, overlap)
		})
	}
}

func TestSchedulesOverlap(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		s1        string
		s2        string
		overlap   bool
		expectErr bool
	}{
		{
			name:    "Same schedule",
			s1:      "* 9-18 * * 1-5",
			s2:      "* 9-18 * * 1-5",
			overlap: true,
		},
		{
			name:    "Different hours",
			s1:      "* 9-12 * * 1-5",
			s2:      "* 13-18 * * 1-5",
			overlap: false,
		},
		{
			name:    "Different DOW but wildcard DOM",
			s1:      "* 9-18 * * 1-3",
			s2:      "* 9-18 * * 4-5",
			overlap: true, // true because both have wildcard DOM
		},
		{
			name:      "Invalid hour range",
			s1:        "* 25-26 * * 1-5",
			s2:        "* 9-18 * * 1-5",
			expectErr: true,
		},
		{
			name:      "Invalid month range",
			s1:        "* 9-18 13 * 1-5",
			s2:        "* 9-18 * * 1-5",
			expectErr: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			s1, err := cron.Weekly(testCase.s1)
			if testCase.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			s2, err := cron.Weekly(testCase.s2)
			if testCase.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			overlap, err := cron.SchedulesOverlap(s1, s2)
			if testCase.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.overlap, overlap)
		})
	}
}
