package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// backOffToCeiling fails enough attempts for the delay limit to stop growing.
func backOffToCeiling(b *reconnectBackoff) {
	for limit := reconnectBackoffFloor; limit < reconnectBackoffCeil; limit *= 2 {
		b.next(0, 0)
	}
	b.next(0, 0)
}

func TestReconnectBackoff(t *testing.T) {
	t.Parallel()

	t.Run("GrowsFromImmediateToCeiling", func(t *testing.T) {
		t.Parallel()
		var b reconnectBackoff

		require.Zero(t, b.next(0, 0), "a starting agent connects immediately")
		for limit := reconnectBackoffFloor; limit < reconnectBackoffCeil; limit *= 2 {
			require.LessOrEqual(t, b.next(0, 0), limit)
		}

		// The delays must be spread across the whole range, or agents that
		// lost coderd together would come back together. The chance that 100
		// uniform draws all land in the lower half is 2^-100.
		var longest time.Duration
		distinct := make(map[time.Duration]struct{})
		for range 100 {
			delay := b.next(0, 0)
			require.LessOrEqual(t, delay, reconnectBackoffCeil)
			longest = max(longest, delay)
			distinct[delay] = struct{}{}
		}
		require.Greater(t, longest, reconnectBackoffCeil/2)
		require.Greater(t, len(distinct), 1)
	})

	t.Run("HealthyConnectionStartsOver", func(t *testing.T) {
		t.Parallel()
		var b reconnectBackoff
		backOffToCeiling(&b)

		require.LessOrEqual(t, b.next(reconnectHealthyAfter, 0), reconnectBackoffFloor)
		require.LessOrEqual(t, b.next(0, 0), 2*reconnectBackoffFloor)
	})

	t.Run("ShortConnectionKeepsBackingOff", func(t *testing.T) {
		t.Parallel()
		var b reconnectBackoff
		backOffToCeiling(&b)

		// A connection that coderd drops again right away is not a recovery.
		// The chance that 100 draws from the full range all fit under the
		// floor is below 300^-100.
		var longest time.Duration
		for range 100 {
			longest = max(longest, b.next(reconnectHealthyAfter-time.Second, 0))
		}
		require.Greater(t, longest, reconnectBackoffFloor)
	})

	t.Run("RetryAfterIsAFloor", func(t *testing.T) {
		t.Parallel()
		var b reconnectBackoff
		b.next(0, 0)

		delay := b.next(0, 7*time.Second)
		require.GreaterOrEqual(t, delay, 7*time.Second)
		require.LessOrEqual(t, delay, 7*time.Second+reconnectBackoffFloor)

		// A server cannot keep the agent away indefinitely.
		delay = b.next(0, 24*time.Hour)
		require.GreaterOrEqual(t, delay, reconnectMaxRetryAfter)
		require.LessOrEqual(t, delay, reconnectMaxRetryAfter+2*reconnectBackoffFloor)
	})
}
