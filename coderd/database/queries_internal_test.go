package database

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/usage/usagetypes"
)

// TestGetTotalUsageHBAgentRuntimeV1QueryEventType pins the event type literal
// in the generated SQL to the Go constant. The usage_event_type_check
// constraint and the rollup trigger break loudly on writes if the event type
// is ever renamed or versioned, but this read-only predicate would silently
// start returning 0, which is indistinguishable from zero usage at every
// layer above it.
func TestGetTotalUsageHBAgentRuntimeV1QueryEventType(t *testing.T) {
	t.Parallel()

	require.Contains(t, getTotalUsageHBAgentRuntimeV1,
		string(usagetypes.UsageEventTypeHBAgentRuntimeV1))
}
