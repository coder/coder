package chatd

import (
	"context"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/coder/coder/v2/coderd/database"
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

// ManageAutomationsAvailable reports whether a turn of chat offers the
// manage_automations tool.
func (p *Server) ManageAutomationsAvailable(ctx context.Context, chat database.Chat) bool {
	return p.manageAutomationsAvailable(ctx, chat)
}
