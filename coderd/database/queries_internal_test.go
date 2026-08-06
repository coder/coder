package database

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/usage/usagetypes"
)

// TestGetTotalUsageHBAgentRuntimeV1QueryEventType pins the event type and
// payload field literals in the generated SQL to the Go producer. The
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
	for field := range (usagetypes.HBAgentRuntime{}).Fields() {
		require.Contains(t, getTotalUsageHBAgentRuntimeV1, field)
	}
}
