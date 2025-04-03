package httpmw

import (
	"context"
	"net/http"
	"testing"
	"time"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
)

func TestRequestLoggerContext_WriteLog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	testLogger := slogtest.Make(t, nil)
	startTime := time.Now()

	logCtx := NewRequestLoggerContext(testLogger, "GET", startTime)

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
