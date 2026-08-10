package database

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/usage/usagetypes"
)

// TestGetTotalUsageHBAgentRuntimeV1QueryEventType pins the event type and
// payload extraction literals in the generated SQL to the Go producer.
// Renaming either would make this read-only query silently return 0 (->> on
// a missing key yields NULL, SUM skips NULLs, COALESCE reports 0), which is
// indistinguishable from zero usage at every layer above it.
func TestGetTotalUsageHBAgentRuntimeV1QueryEventType(t *testing.T) {
	t.Parallel()

	require.Contains(t, getTotalUsageHBAgentRuntimeV1,
		string(usagetypes.UsageEventTypeHBAgentRuntimeV1))
	// The full extraction expression is pinned, not the bare key: the
	// query's result alias (total_runtime_ms) contains "runtime_ms", so a
	// bare-key assertion would keep passing after the ->> key was renamed.
	for field := range (usagetypes.HBAgentRuntime{}).Fields() {
		require.Contains(t, getTotalUsageHBAgentRuntimeV1,
			"event_data->>'"+field+"'")
	}
}
