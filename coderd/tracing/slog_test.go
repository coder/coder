package tracing_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/tracing"
)

type stringer string

var _ fmt.Stringer = stringer("")

func (s stringer) String() string {
	return string(s)
}

type traceEvent struct {
	name       string
	attributes []attribute.KeyValue
}

type slogFakeSpan struct {
	trace.Span // always nil

	isRecording bool
	events      []traceEvent
}

// We overwrite some methods below.
var _ trace.Span = &slogFakeSpan{}

// IsRecording implements trace.Span.
func (s *slogFakeSpan) IsRecording() bool {
	return s.isRecording
}

// AddEvent implements trace.Span.
func (s *slogFakeSpan) AddEvent(name string, options ...trace.EventOption) {
	cfg := trace.NewEventConfig(options...)

	s.events = append(s.events, traceEvent{
		name:       name,
		attributes: cfg.Attributes(),
	})
}

func Test_SlogSink(t *testing.T) {
	t.Parallel()

	fieldsMap := map[string]interface{}{
		"test_bool":      true,
		"test_[]bool":    []bool{true, false},
		"test_float32":   float32(1.1),
		"test_float64":   float64(1.1),
		"test_[]float64": []float64{1.1, 2.2},
		"test_int":       int(1),
		"test_[]int":     []int{1, 2},
		"test_int8":      int8(1),
		"test_int16":     int16(1),
		"test_int32":     int32(1),
		"test_int64":     int64(1),
		"test_[]int64":   []int64{1, 2},
		"test_uint":      uint(1),
		"test_uint8":     uint8(1),
		"test_uint16":    uint16(1),
		"test_uint32":    uint32(1),
		"test_uint64":    uint64(1),
		"test_string":    "test",
		"test_[]string":  []string{"test1", "test2"},
		"test_duration":  time.Second,
		"test_time":      time.Now(),
		"test_stringer":  stringer("test"),
		"test_struct": struct {
			Field string `json:"field"`
		}{
			Field: "test",
		},
	}

	entry := slog.SinkEntry{
		Time:        time.Now(),
		Level:       slog.LevelInfo,
		Message:     "hello",
		LoggerNames: []string{"foo", "bar"},
		Func:        "hello",
		File:        "hello.go",
		Line:        42,
		Fields:      mapToSlogFields(fieldsMap),
	}

	t.Run("NotRecording", func(t *testing.T) {
		t.Parallel()

		sink := tracing.SlogSink{}
		span := &slogFakeSpan{
			isRecording: false,
		}
		ctx := trace.ContextWithSpan(context.Background(), span)

		sink.LogEntry(ctx, entry)
		require.Len(t, span.events, 0)
	})

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		sink := tracing.SlogSink{}
		sink.Sync()

		span := &slogFakeSpan{
			isRecording: true,
		}
		ctx := trace.ContextWithSpan(context.Background(), span)

		sink.LogEntry(ctx, entry)
		require.Len(t, span.events, 1)

		sink.LogEntry(ctx, entry)
		require.Len(t, span.events, 2)

		e := span.events[0]
		require.Equal(t, "log: INFO: hello", e.name)

		expectedAttributes := mapToBasicMap(fieldsMap)
		delete(expectedAttributes, "test_struct")
		expectedAttributes["slog.time"] = entry.Time.Format(time.RFC3339Nano)
		expectedAttributes["slog.logger"] = strings.Join(entry.LoggerNames, ".")
		expectedAttributes["slog.level"] = entry.Level.String()
		expectedAttributes["slog.message"] = entry.Message
		expectedAttributes["slog.func"] = entry.Func
		expectedAttributes["slog.file"] = entry.File
		expectedAttributes["slog.line"] = int64(entry.Line)

		require.Equal(t, expectedAttributes, attributesToMap(e.attributes))
	})
}

func mapToSlogFields(m map[string]interface{}) slog.Map {
	fields := make(slog.Map, 0, len(m))
	for k, v := range m {
		fields = append(fields, slog.F(k, v))
	}

	return fields
}

func mapToBasicMap(m map[string]interface{}) map[string]interface{} {
	basic := make(map[string]interface{}, len(m))
	for k, v := range m {
		var val interface{} = v
		switch v := v.(type) {
		case float32:
			val = float64(v)
		case int:
			val = int64(v)
		case []int:
			i64Slice := make([]int64, len(v))
			for i, v := range v {
				i64Slice[i] = int64(v)
			}
			val = i64Slice
		case int8:
			val = int64(v)
		case int16:
			val = int64(v)
		case int32:
			val = int64(v)
		case uint:
			// #nosec G115 - Safe conversion for test data
			val = int64(v)
		case uint8:
			val = int64(v)
		case uint16:
			val = int64(v)
		case uint32:
			val = int64(v)
		case uint64:
			// #nosec G115 - Safe conversion for test data with small test values
			val = int64(v)
		case time.Duration:
			val = v.String()
		case time.Time:
			val = v.Format(time.RFC3339Nano)
		case fmt.Stringer:
			val = v.String()
		}

		basic[k] = val
	}

	return basic
}

func attributesToMap(attrs []attribute.KeyValue) map[string]interface{} {
	m := make(map[string]interface{}, len(attrs))
	for _, attr := range attrs {
		m[string(attr.Key)] = attr.Value.AsInterface()
	}

	return m
}
