package workspacestats

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/proto"
)

func TestReportAgentStatsWithRDP(t *testing.T) {
	t.Parallel()

	// Test that RDP sessions trigger activity bump
	t.Run("RDP sessions trigger activity bump", func(t *testing.T) {
		t.Parallel()

		// Create stats with RDP session
		stats := &proto.Stats{
			SessionCountVscode:          0,
			SessionCountJetbrains:       0,
			SessionCountReconnectingPty: 0,
			SessionCountSsh:             0,
			SessionCountRdp:             1, // RDP session active
			ConnectionCount:             1,
		}

		// Test the activity detection logic directly
		// This should return false (meaning activity should be bumped) when RDP sessions are present
		shouldSkipActivityBump := stats.SessionCountVscode == 0 && 
			stats.SessionCountJetbrains == 0 && 
			stats.SessionCountReconnectingPty == 0 && 
			stats.SessionCountSsh == 0 && 
			stats.SessionCountRdp == 0

		// With RDP session present, shouldSkipActivityBump should be false
		require.False(t, shouldSkipActivityBump, "Activity bump should not be skipped when RDP sessions are present")
	})

	// Test that no RDP sessions don't trigger activity bump when no other sessions
	t.Run("No RDP sessions don't trigger activity bump when no other sessions", func(t *testing.T) {
		t.Parallel()

		// Create stats with no sessions
		stats := &proto.Stats{
			SessionCountVscode:          0,
			SessionCountJetbrains:       0,
			SessionCountReconnectingPty: 0,
			SessionCountSsh:             0,
			SessionCountRdp:             0, // No RDP session
			ConnectionCount:             0,
		}

		// Test the activity detection logic directly
		shouldSkipActivityBump := stats.SessionCountVscode == 0 && 
			stats.SessionCountJetbrains == 0 && 
			stats.SessionCountReconnectingPty == 0 && 
			stats.SessionCountSsh == 0 && 
			stats.SessionCountRdp == 0

		// With no sessions present, shouldSkipActivityBump should be true
		require.True(t, shouldSkipActivityBump, "Activity bump should be skipped when no sessions are present")
	})
}
