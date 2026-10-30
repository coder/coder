package chatd

import (
	"maps"
	"slices"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// StageAnomalyCount returns the stage anomaly count for reason.
var StageAnomalyCount = anomalyCount

// DefaultTaskTimeout is the per-attempt task timeout.
const DefaultTaskTimeout = defaultTaskTimeout

// Workspace awareness texts that turns show the model.
const (
	WorkspaceAttachedAwareness = workspaceAttachedAwareness
	WorkspaceDetachedAwareness = workspaceDetachedAwareness
)

// LegacyWorkspaceAwarenessTexts returns the workspace awareness texts that
// chat creation used to save, sorted.
func LegacyWorkspaceAwarenessTexts() []string {
	return slices.Sorted(maps.Keys(legacyWorkspaceAwarenessTexts))
}

// SpanAttr returns the span attribute key, or "" when unset.
func SpanAttr(t *testing.T, span sdktrace.ReadOnlySpan, key string) string {
	t.Helper()
	value, ok := spanAttribute(t, span, key)
	if !ok {
		return ""
	}
	return value.Emit()
}
