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

func TestListMissingChatMessageRuntimeBucketsQueryLiterals(t *testing.T) {
	t.Parallel()

	require.Contains(t, listMissingChatMessageRuntimeBuckets,
		string(usagetypes.UsageEventTypeHBAgentRuntimeV1))
	for field := range (usagetypes.HBAgentRuntime{}).Fields() {
		require.Contains(t, listMissingChatMessageRuntimeBuckets,
			"SUM(cm."+field+")")
	}
}

func TestAgentRuntimeBackfillCheckpointQueryLiterals(t *testing.T) {
	t.Parallel()

	const (
		checkpointKey     = "agent_runtime_all_history_catchup_v1"
		pendingCheckpoint = `{"version":1,"status":"pending"}`
	)
	require.Contains(t, ensureAgentRuntimeBackfillCheckpoint, checkpointKey)
	require.Contains(t, ensureAgentRuntimeBackfillCheckpoint, pendingCheckpoint)
	require.Contains(t, getAgentRuntimeBackfillCheckpoint, checkpointKey)
	require.Contains(t, updateAgentRuntimeBackfillCheckpoint, checkpointKey)
}
