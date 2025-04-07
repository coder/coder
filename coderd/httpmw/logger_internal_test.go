package httpmw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
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

	require.Len(t, sink.entries, 1, "log was written twice")

	require.Equal(t, sink.entries[0].Message, "GET", "log message should be GET")

	require.Equal(t, sink.entries[0].Fields[0].Value, "custom_value", "custom_field should be custom_value")

	// Attempt to write again (should be skipped).
	logCtx.WriteLog(ctx, http.StatusInternalServerError)

	require.Len(t, sink.entries, 1, "log was written twice")
}

func TestLoggerMiddleware_SingleRequest(t *testing.T) {
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

	require.Len(t, sink.entries, 1, "log was written twice")

	require.Equal(t, sink.entries[0].Message, "GET", "log message should be GET")
}

func TestLoggerMiddleware_WebSocket(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	sink := &fakeSink{}
	logger := slog.Make(sink)
	logger = logger.Leveled(slog.LevelDebug)
	wg := sync.WaitGroup{}
	// Create a test handler to simulate a WebSocket connection
	testHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(rw, r, nil)
		if err != nil {
			t.Errorf("failed to accept websocket: %v", err)
			return
		}
		requestLgr := RequestLoggerFromContext(r.Context())
		requestLgr.WriteLog(r.Context(), http.StatusSwitchingProtocols)
		wg.Done()
		defer conn.Close(websocket.StatusNormalClosure, "")

		// Send a couple of messages for testing
		_ = conn.Write(ctx, websocket.MessageText, []byte("ping"))
		_ = conn.Write(ctx, websocket.MessageText, []byte("pong"))
	})

	// Wrap the test handler with the Logger middleware
	loggerMiddleware := Logger(logger)
	wrappedHandler := loggerMiddleware(testHandler)

	// RequestLogger expects the ResponseWriter to be *tracing.StatusWriter
	customHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		sw := &tracing.StatusWriter{ResponseWriter: rw}
		wrappedHandler.ServeHTTP(sw, r)
	})

	srv := httptest.NewServer(customHandler)
	defer srv.Close()
	wg.Add(1)
	// nolint: bodyclose
	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	if err != nil {
		t.Fatalf("failed to create WebSocket connection: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	wg.Wait()
	require.Len(t, sink.entries, 1, "log was written twice")

	require.Equal(t, sink.entries[0].Message, "GET", "log message should be GET")
}

type fakeSink struct {
	entries []slog.SinkEntry
}

func (s *fakeSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.entries = append(s.entries, e)
}

func (*fakeSink) Sync() {}
