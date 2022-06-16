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
		testCase := testCase
		t.Run(testCase.Duration, func(t *testing.T) {
			t.Parallel()
			d, err := time.ParseDuration(testCase.Duration)
			require.NoError(t, err)
			actual := durationDisplay(d)
			assert.Equal(t, testCase.Expected, actual)
		})
	}
}

func TestRelative(t *testing.T) {
	t.Parallel()
	assert.Equal(t, relative(time.Minute), "in 1m")
	assert.Equal(t, relative(-time.Minute), "1m ago")
	assert.Equal(t, relative(0), "now")
}
