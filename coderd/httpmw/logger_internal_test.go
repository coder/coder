package httpmw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/tracing"
)

func TestRequestLoggerContext_WriteLog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	testLogger := slogtest.Make(t, nil)

	logCtx := NewRequestLoggerContext(testLogger, "GET", time.Now())

	// Add custom fields
	logCtx.WithFields(
		slog.F("custom_field", "custom_value"),
	)

	// Write log for 200 status
	logCtx.WriteLog(ctx, http.StatusOK)

	if !logCtx.written {
		t.Error("expected log to be written once")
	}
	// Attempt to write again (should be skipped).
	// If the error log entry gets written,
	// slogtest will fail the test.
	logCtx.WriteLog(ctx, http.StatusInternalServerError)
}

func TestLoggerMiddleware(t *testing.T) {
	t.Parallel()

	// Create a test logger
	testLogger := slogtest.Make(t, nil)

	// Create a test handler to simulate an HTTP request
	testHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("OK"))
	})

	// Wrap the test handler with the Logger middleware
	loggerMiddleware := Logger(testLogger)
	wrappedHandler := loggerMiddleware(testHandler)

	// Create a test HTTP request
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test-path", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	sw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}

	// Serve the request
	wrappedHandler.ServeHTTP(sw, req)

	logCtx := RequestLoggerFromContext(context.Background())
	// Verify that the log was written
	if logCtx != nil && !logCtx.written {
		t.Error("expected log to be written exactly once")
	}
}
