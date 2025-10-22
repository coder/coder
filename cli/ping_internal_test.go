package cli

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"tailscale.com/ipn/ipnstate"
)

func TestBuildSummary(t *testing.T) {
	t.Parallel()

	t.Run("Ok", func(t *testing.T) {
		t.Parallel()
		input := []*ipnstate.PingResult{
			{
				Err:            "",
				LatencySeconds: 0.1,
			},
			{
				Err:            "",
				LatencySeconds: 0.3,
			},
			{
				Err:            "",
				LatencySeconds: 0.2,
			},
			{
				Err:            "ping error",
				LatencySeconds: 0.4,
			},
		}

		actual := pingSummary{
			Workspace: "test",
		}
		for _, r := range input {
			actual.addResult(r)
		}
		actual.Write(io.Discard)
		require.Equal(t, time.Duration(0.1*float64(time.Second)), *actual.Min)
		require.Equal(t, time.Duration(0.2*float64(time.Second)), *actual.Avg)
		require.Equal(t, time.Duration(0.3*float64(time.Second)), *actual.Max)
		require.Equal(t, time.Duration(0.009999999*float64(time.Second)), *actual.Variance)
		require.Equal(t, actual.Successful, 3)
	})

	t.Run("One", func(t *testing.T) {
		t.Parallel()
		input := []*ipnstate.PingResult{
			{
				LatencySeconds: 0.2,
			},
		}

		actual := &pingSummary{
			Workspace: "test",
		}
		for _, r := range input {
			actual.addResult(r)
		}
		actual.Write(io.Discard)
		require.Equal(t, actual.Successful, 1)
		require.Equal(t, time.Duration(0.2*float64(time.Second)), *actual.Min)
		require.Equal(t, time.Duration(0.2*float64(time.Second)), *actual.Avg)
		require.Equal(t, time.Duration(0.2*float64(time.Second)), *actual.Max)
		require.Nil(t, actual.Variance)
	})

	t.Run("NoLatency", func(t *testing.T) {
		t.Parallel()
		input := []*ipnstate.PingResult{
			{
				Err: "ping error",
			},
			{
				Err:            "ping error",
				LatencySeconds: 0.2,
			},
		}

		expected := &pingSummary{
			Workspace:  "test",
			Total:      2,
			Successful: 0,
			Min:        nil,
			Avg:        nil,
			Max:        nil,
			Variance:   nil,
			latencySum: 0,
			runningAvg: 0,
			m2:         0,
		}

		actual := &pingSummary{
			Workspace: "test",
		}
		for _, r := range input {
			actual.addResult(r)
		}
		actual.Write(io.Discard)
		require.Equal(t, expected, actual)
	})
}
