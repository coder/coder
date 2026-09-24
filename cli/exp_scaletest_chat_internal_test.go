package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTrimmedDurationSet(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   string
		want time.Duration
	}{
		{"5s", 5 * time.Second},
		{"5s ", 5 * time.Second},
		{" 5s", 5 * time.Second},
		{"\t3s\n", 3 * time.Second},
		{"0s", 0},
		{"1m30s", 90 * time.Second},
	} {
		var d time.Duration
		require.NoError(t, trimmedDurationOf(&d).Set(tt.in), "input %q", tt.in)
		require.Equal(t, tt.want, d, "input %q", tt.in)
	}

	// A bare integer without a unit is still rejected.
	var d time.Duration
	require.Error(t, trimmedDurationOf(&d).Set("5"))
}
