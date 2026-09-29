package chatd

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
)

// StageAnomalyCount returns the stage anomaly count for reason.
var StageAnomalyCount = anomalyCount

// DefaultTaskTimeout is the per-attempt task timeout.
const DefaultTaskTimeout = defaultTaskTimeout

// TurnCategorySeconds returns a root chat's turn time for category and outcome.
var TurnCategorySeconds = turnCategorySeconds

// RootTurnOutcomeCount returns a root chat's turn count for outcome.
func RootTurnOutcomeCount(t *testing.T, registry *prometheus.Registry, outcome chatloop.TurnOutcome) float64 {
	t.Helper()
	return turnOutcomeCount(t, registry, chatloop.ChatKindRoot, outcome)
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
