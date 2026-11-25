package chattest

import (
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// SpanStage returns the chat lifecycle stage a span records, or "" when it
// has none. GenAI spans are named after their operation, so tests identify
// stages by this attribute. The key matches chatloop.AttrStage, which this
// package cannot import because chatloop's internal tests import it.
func SpanStage(span sdktrace.ReadOnlySpan) string {
	for _, attr := range span.Attributes() {
		if attr.Key == "stage" {
			return attr.Value.AsString()
		}
	}
	return ""
}
