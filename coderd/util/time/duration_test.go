package duration_test

import (
	"testing"
	"time"

	duration "github.com/coder/coder/v2/coderd/util/time"
)

func TestHumanize(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		duration time.Duration
		expected string
	}{
		{
			duration: time.Duration(0),
			expected: "0 seconds",
		},
		{
			duration: time.Duration(1 * time.Second),
			expected: "1 second",
		},
		{
			duration: time.Duration(45 * time.Second),
			expected: "45 seconds",
		},
		{
			duration: time.Duration(30 * time.Minute),
			expected: "30 minutes",
		},
		{
			duration: time.Duration(30*time.Minute + 10*time.Second),
			expected: "30 minutes and 10 seconds",
		},
		{
			duration: time.Duration(2*time.Hour + 25*time.Minute + 10*time.Second),
			expected: "2 hours, 25 minutes and 10 seconds",
		},
		{
			duration: time.Duration(2400 * time.Hour),
			expected: "100 days",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			t.Parallel()

			actual := duration.Humanize(tc.duration)
			if actual != tc.expected {
				t.Errorf("expected: %s, got: %s", tc.expected, actual)
			}
		})
	}
}
