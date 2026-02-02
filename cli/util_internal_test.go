package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDurationDisplay(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		Duration string
		Expected string
	}{
		{"-1s", "<1m"},
		{"0s", "0s"},
		{"1s", "<1m"},
		{"59s", "<1m"},
		{"1m", "1m"},
		{"1m1s", "1m"},
		{"2m", "2m"},
		{"59m", "59m"},
		{"1h", "1h"},
		{"1h1m1s", "1h1m"},
		{"2h", "2h"},
		{"23h", "23h"},
		{"24h", "1d"},
		{"24h1m1s", "1d"},
		{"25h", "1d1h"},
	} {
		t.Run(testCase.Duration, func(t *testing.T) {
			t.Parallel()
			d, err := time.ParseDuration(testCase.Duration)
			require.NoError(t, err)
			actual := durationDisplay(d)
			assert.Equal(t, testCase.Expected, actual)
		})
	}
}

func TestExtendedParseDuration(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		Duration   string
		Expected   time.Duration
		ExpectedOk bool
	}{
		{"1d", 24 * time.Hour, true},
		{"1y", 365 * 24 * time.Hour, true},
		{"10s", 10 * time.Second, true},
		{"1m", 1 * time.Minute, true},
		{"20h", 20 * time.Hour, true},
		{"10y10d10s", 10*365*24*time.Hour + 10*24*time.Hour + 10*time.Second, true},
		{"10ms", 10 * time.Millisecond, true},
		{"5y10d10s5y2ms8ms", 10*365*24*time.Hour + 10*24*time.Hour + 10*time.Second + 10*time.Millisecond, true},
		{"10yz10d10s", 0, false},
		{"1µs2h1d", 1*time.Microsecond + 2*time.Hour + 1*24*time.Hour, true},
		{"1y365d", 2 * 365 * 24 * time.Hour, true},
		{"1µs10us", 1*time.Microsecond + 10*time.Microsecond, true},
		// negative related tests
		{"-", 0, false},
		{"-2h10m", -2*time.Hour - 10*time.Minute, true},
		{"--10s", 0, false},
		{"10s-10m", 0, false},
		// overflow related tests
		{"-20000000000000h", 0, false},
		{"92233754775807y", 0, false},
		{"200y200y200y200y200y", 0, false},
		{"9223372036854775807s", 0, false},
		// fractional values
		{"1.5d", 36 * time.Hour, true},
		{"0.5h", 30 * time.Minute, true},
		{"2.5s", 2500 * time.Millisecond, true},
		{"1.5y", time.Duration(float64(365*24*time.Hour) * 1.5), true},
		{"0.5m", 30 * time.Second, true},
		{"1.5h30m", 2 * time.Hour, true},
		{"0.25d", 6 * time.Hour, true},
		{"-1.5h", -90 * time.Minute, true},
		{"100.5ms", 100*time.Millisecond + 500*time.Microsecond, true},
		{"1.5us", 1500 * time.Nanosecond, true},
		{"1.5µs", 1500 * time.Nanosecond, true},
	} {
		t.Run(testCase.Duration, func(t *testing.T) {
			t.Parallel()
			actual, err := extendedParseDuration(testCase.Duration)
			if testCase.ExpectedOk {
				require.NoError(t, err)
				assert.Equal(t, testCase.Expected, actual)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestRelative(t *testing.T) {
	t.Parallel()
	assert.Equal(t, relative(time.Minute), "in 1m")
	assert.Equal(t, relative(-time.Minute), "1m ago")
	assert.Equal(t, relative(0), "now")
}
