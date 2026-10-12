package chatd

import (
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// StageAnomalyCount returns the stage anomaly count for reason.
var StageAnomalyCount = anomalyCount

// DefaultTaskTimeout is the per-attempt task timeout.
const DefaultTaskTimeout = defaultTaskTimeout

// SpanAttr returns the span attribute key, or "" when unset.
func SpanAttr(t *testing.T, span sdktrace.ReadOnlySpan, key string) string {
	t.Helper()
	value, ok := spanAttribute(t, span, key)
	if !ok {
		return ""
	}
	return value.Emit()
}
