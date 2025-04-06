package httpmw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/tracing"
)

func TestRequestLogger_WriteLog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	sink := &fakeSink{}
	logger := slog.Make(sink)
	logger = logger.Leveled(slog.LevelDebug)
	logCtx := NewRequestLogger(logger, "GET", time.Now())

	// Add custom fields
	logCtx.WithFields(
		slog.F("custom_field", "custom_value"),
	)

	// Write log for 200 status
	logCtx.WriteLog(ctx, http.StatusOK)

	if len(sink.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(sink.entries))
	}

	if sink.entries[0].Message != "GET" {
		t.Errorf("expected log message to be 'GET', got '%s'", sink.entries[0].Message)
	}

	if sink.entries[0].Fields[0].Value != "custom_value" {
		t.Errorf("expected a custom_field with value custom_value, got '%s'", sink.entries[0].Fields[0].Value)
	}

	// Attempt to write again (should be skipped).
	logCtx.WriteLog(ctx, http.StatusInternalServerError)

	if len(sink.entries) != 1 {
		t.Fatalf("expected 1 log entry after second write, got %d", len(sink.entries))
	}
}

func TestLoggerMiddleware(t *testing.T) {
	t.Parallel()

	sink := &fakeSink{}
	logger := slog.Make(sink)
	logger = logger.Leveled(slog.LevelDebug)

	// Create a test handler to simulate an HTTP request
	testHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("OK"))
	})

	// Wrap the test handler with the Logger middleware
	loggerMiddleware := Logger(logger)
	wrappedHandler := loggerMiddleware(testHandler)

	// Create a test HTTP request
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test-path", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	sw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}

	// Serve the request
	wrappedHandler.ServeHTTP(sw, req)

	if len(sink.entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(sink.entries))
	}

	if sink.entries[0].Message != "GET" {
		t.Errorf("expected log message to be 'GET', got '%s'", sink.entries[0].Message)
	}
}

type fakeSink struct {
	entries []slog.SinkEntry
}

func (s *fakeSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.entries = append(s.entries, e)
}

func (*fakeSink) Sync() {}
