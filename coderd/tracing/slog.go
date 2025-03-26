package tracing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog"
)

type SlogSink struct{}

var _ slog.Sink = SlogSink{}

// LogEntry implements slog.Sink. All entries are added as events to the span
// in the context. If no span is present, the entry is dropped.
func (SlogSink) LogEntry(ctx context.Context, e slog.SinkEntry) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		// If the span is a noopSpan or isn't recording, we don't want to
		// compute the attributes (which is expensive) below.
		return
	}

	attributes := []attribute.KeyValue{
		attribute.String("slog.time", e.Time.Format(time.RFC3339Nano)),
		attribute.String("slog.logger", strings.Join(e.LoggerNames, ".")),
		attribute.String("slog.level", e.Level.String()),
		attribute.String("slog.message", e.Message),
		attribute.String("slog.func", e.Func),
		attribute.String("slog.file", e.File),
		attribute.Int64("slog.line", int64(e.Line)),
	}
	attributes = append(attributes, slogFieldsToAttributes(e.Fields)...)

	name := fmt.Sprintf("log: %s: %s", e.Level, e.Message)
	span.AddEvent(name, trace.WithAttributes(attributes...))
}

// Sync implements slog.Sink. No-op as syncing is handled externally by otel.
func (SlogSink) Sync() {}

func slogFieldsToAttributes(m slog.Map) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(m))
	for _, f := range m {
		var value attribute.Value
		switch v := f.Value.(type) {
		case bool:
			value = attribute.BoolValue(v)
		case []bool:
			value = attribute.BoolSliceValue(v)
		case float32:
			value = attribute.Float64Value(float64(v))
		// no float32 slice method
		case float64:
			value = attribute.Float64Value(v)
		case []float64:
			value = attribute.Float64SliceValue(v)
		case int:
			value = attribute.Int64Value(int64(v))
		case []int:
			value = attribute.IntSliceValue(v)
		case int8:
			value = attribute.Int64Value(int64(v))
		// no int8 slice method
		case int16:
			value = attribute.Int64Value(int64(v))
		// no int16 slice method
		case int32:
			value = attribute.Int64Value(int64(v))
		// no int32 slice method
		case int64:
			value = attribute.Int64Value(v)
		case []int64:
			value = attribute.Int64SliceValue(v)
		case uint:
			// #nosec G115 - Safe conversion from uint to int64 as we're only using this for non-critical logging/tracing
			value = attribute.Int64Value(int64(v))
		// no uint slice method
		case uint8:
			value = attribute.Int64Value(int64(v))
		// no uint8 slice method
		case uint16:
			value = attribute.Int64Value(int64(v))
		// no uint16 slice method
		case uint32:
			value = attribute.Int64Value(int64(v))
		// no uint32 slice method
		case uint64:
			// #nosec G115 - Safe conversion from uint64 to int64 as we're only using this for non-critical logging/tracing
			// This is intentionally lossy for very large values, but acceptable for tracing purposes
			value = attribute.Int64Value(int64(v))
		// no uint64 slice method
		case string:
			value = attribute.StringValue(v)
		case []string:
			value = attribute.StringSliceValue(v)
		case time.Duration:
			value = attribute.StringValue(v.String())
		case time.Time:
			value = attribute.StringValue(v.Format(time.RFC3339Nano))
		case fmt.Stringer:
			value = attribute.StringValue(v.String())
		}

		if value.Type() != attribute.INVALID {
			attrs = append(attrs, attribute.KeyValue{
				Key:   attribute.Key(f.Name),
				Value: value,
			})
		}
	}

	return attrs
}
