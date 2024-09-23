package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"tailscale.com/ipn/ipnstate"

	"github.com/coder/coder/v2/coderd/util/ptr"
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
				LatencySeconds: 0.2,
			},
			{
				Err:            "",
				LatencySeconds: 0.3,
			},
			{
				Err:            "ping error",
				LatencySeconds: 0.4,
			},
		}

		expected := &pingSummary{
			Workspace:  "test",
			Total:      4,
			Successful: 3,
			Min:        ptr.Ref(time.Duration(0.1 * float64(time.Second))),
			Avg:        ptr.Ref(time.Duration(0.2 * float64(time.Second))),
			Max:        ptr.Ref(time.Duration(0.3 * float64(time.Second))),
			StdDev:     ptr.Ref(time.Duration(0.081649658 * float64(time.Second))),
		}

		actual := buildSummary("test", input)
		require.Equal(t, expected, actual)
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
			StdDev:     nil,
		}

		actual := buildSummary("test", input)
		require.Equal(t, expected, actual)
	})
}
