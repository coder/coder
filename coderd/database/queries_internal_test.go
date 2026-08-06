package database

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/usage/usagetypes"
)

// TestGetTotalUsageHBAgentRuntimeV1QueryEventType pins the event type and
// payload extraction literals in the generated SQL to the Go producer. The
// usage_event_type_check constraint and the rollup trigger break loudly on
// writes if the event type is ever renamed or versioned, but this read-only
// predicate would silently start returning 0, which is indistinguishable
// from zero usage at every layer above it. The same silent zero happens if
// HBAgentRuntime.Fields ever renames runtime_ms: ->> on a missing key
// yields NULL, SUM skips NULLs, and COALESCE reports 0.
func TestGetTotalUsageHBAgentRuntimeV1QueryEventType(t *testing.T) {
	t.Parallel()

	require.Contains(t, getTotalUsageHBAgentRuntimeV1,
		string(usagetypes.UsageEventTypeHBAgentRuntimeV1))
	// The full extraction expression is pinned, not the bare key: the
	// query's result alias (total_runtime_ms) contains "runtime_ms", so a
	// bare-key assertion would keep passing after the ->> key was renamed.
	// The query reads exactly the producer's payload, which today is the
	// single runtime_ms field; if HBAgentRuntime ever grows a field this
	// query intentionally does not read, this loop needs an allowlist
	// rather than a weaker assertion.
	for field := range (usagetypes.HBAgentRuntime{}).Fields() {
		require.Contains(t, getTotalUsageHBAgentRuntimeV1,
			"event_data->>'"+field+"'")
	}
}
