package chatd

import (
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// StageAnomalyCount returns the coderd_chatd_stage_anomalies_total
// count recorded for reason.
var StageAnomalyCount = anomalyCount

// ErrChatInterrupted is the error an interrupted chat_turn span ends
// with.
var ErrChatInterrupted = errChatInterrupted

// DefaultTaskTimeout is how long a task attempt runs before the task
// timeout cancels it.
const DefaultTaskTimeout = defaultTaskTimeout

// SpanAttr returns the emitted value of the span attribute key, or an
// empty string when the span does not carry it.
func SpanAttr(t *testing.T, span sdktrace.ReadOnlySpan, key string) string {
	t.Helper()
	value, ok := spanAttribute(t, span, key)
	if !ok {
		return ""
	}
	return value.Emit()
}
