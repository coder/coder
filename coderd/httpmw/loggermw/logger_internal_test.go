package loggermw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
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

	require.Equal(t, sink.entries[0].Message, "GET")

	require.Equal(t, sink.entries[0].Fields[0].Value, "custom_value")

	// Attempt to write again (should be skipped).
	logCtx.WriteLog(ctx, http.StatusInternalServerError)

	require.Len(t, sink.entries, 1, "log was written twice")
}

func TestLoggerMiddleware_SingleRequest(t *testing.T) {
	t.Parallel()

	sink := &fakeSink{}
	logger := slog.Make(sink)
	logger = logger.Leveled(slog.LevelDebug)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// Create a test handler to simulate an HTTP request
	testHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("OK"))
	})

	// Wrap the test handler with the Logger middleware
	loggerMiddleware := Logger(logger)
	wrappedHandler := loggerMiddleware(testHandler)

	// Create a test HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/test-path", nil)
	require.NoError(t, err, "failed to create request")

	sw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}

	// Serve the request
	wrappedHandler.ServeHTTP(sw, req)

	require.Len(t, sink.entries, 1, "log was written twice")

	require.Equal(t, sink.entries[0].Message, "GET")

	fieldsMap := make(map[string]any)
	for _, field := range sink.entries[0].Fields {
		fieldsMap[field.Name] = field.Value
	}

	// Check that the log contains the expected fields
	requiredFields := []string{"host", "path", "proto", "remote_addr", "start", "took", "status_code", "latency_ms"}
	for _, field := range requiredFields {
		_, exists := fieldsMap[field]
		require.True(t, exists, "field %q is missing in log fields", field)
	}

	require.Len(t, sink.entries[0].Fields, len(requiredFields), "log should contain only the required fields")

	// Check value of the status code
	require.Equal(t, fieldsMap["status_code"], http.StatusOK)
}

func TestLoggerMiddleware_WebSocket(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	sink := &fakeSink{
		newEntries: make(chan slog.SinkEntry, 2),
	}
	logger := slog.Make(sink)
	logger = logger.Leveled(slog.LevelDebug)
	done := make(chan struct{})
	wg := sync.WaitGroup{}
	// Create a test handler to simulate a WebSocket connection
	testHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(rw, r, nil)
		if !assert.NoError(t, err, "failed to accept websocket") {
			return
		}
		defer conn.Close(websocket.StatusGoingAway, "")

		requestLgr := RequestLoggerFromContext(r.Context())
		requestLgr.WriteLog(r.Context(), http.StatusSwitchingProtocols)
		// Block so we can be sure the end of the middleware isn't being called.
		wg.Wait()
	})

	// Wrap the test handler with the Logger middleware
	loggerMiddleware := Logger(logger)
	wrappedHandler := loggerMiddleware(testHandler)

	// RequestLogger expects the ResponseWriter to be *tracing.StatusWriter
	customHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		defer close(done)
		sw := &tracing.StatusWriter{ResponseWriter: rw}
		wrappedHandler.ServeHTTP(sw, r)
	})

	srv := httptest.NewServer(customHandler)
	defer srv.Close()
	wg.Add(1)
	// nolint: bodyclose
	conn, _, err := websocket.Dial(ctx, srv.URL, nil)
	require.NoError(t, err, "failed to dial WebSocket")
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Wait for the log from within the handler
	newEntry := testutil.RequireReceive(ctx, t, sink.newEntries)
	require.Equal(t, newEntry.Message, "GET")

	// Signal the websocket handler to return (and read to handle the close frame)
	wg.Done()
	_, _, err = conn.Read(ctx)
	require.ErrorAs(t, err, &websocket.CloseError{}, "websocket read should fail with close error")

	// Wait for the request to finish completely and verify we only logged once
	_ = testutil.RequireReceive(ctx, t, done)
	require.Len(t, sink.entries, 1, "log was written twice")
}

func TestRequestLogger_HTTPRouteParams(t *testing.T) {
	t.Parallel()

	sink := &fakeSink{}
	logger := slog.Make(sink)
	logger = logger.Leveled(slog.LevelDebug)

	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("workspace", "test-workspace")
	chiCtx.URLParams.Add("agent", "test-agent")

	ctx = context.WithValue(ctx, chi.RouteCtxKey, chiCtx)

	// Create a test handler to simulate an HTTP request
	testHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("OK"))
	})

	// Wrap the test handler with the Logger middleware
	loggerMiddleware := Logger(logger)
	wrappedHandler := loggerMiddleware(testHandler)

	// Create a test HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/test-path/}", nil)
	require.NoError(t, err, "failed to create request")

	sw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}

	// Serve the request
	wrappedHandler.ServeHTTP(sw, req)

	fieldsMap := make(map[string]any)
	for _, field := range sink.entries[0].Fields {
		fieldsMap[field.Name] = field.Value
	}

	// Check that the log contains the expected fields
	requiredFields := []string{"workspace", "agent"}
	for _, field := range requiredFields {
		_, exists := fieldsMap["params_"+field]
		require.True(t, exists, "field %q is missing in log fields", field)
	}
}

func TestRequestLogger_RouteParamsLogging(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		params         map[string]string
		expectedFields []string
	}{
		{
			name:           "EmptyParams",
			params:         map[string]string{},
			expectedFields: []string{},
		},
		{
			name: "SingleParam",
			params: map[string]string{
				"workspace": "test-workspace",
			},
			expectedFields: []string{"params_workspace"},
		},
		{
			name: "MultipleParams",
			params: map[string]string{
				"workspace": "test-workspace",
				"agent":     "test-agent",
				"user":      "test-user",
			},
			expectedFields: []string{"params_workspace", "params_agent", "params_user"},
		},
		{
			name: "EmptyValueParam",
			params: map[string]string{
				"workspace": "test-workspace",
				"agent":     "",
			},
			expectedFields: []string{"params_workspace"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sink := &fakeSink{}
			logger := slog.Make(sink)
			logger = logger.Leveled(slog.LevelDebug)

			// Create a route context with the test parameters
			chiCtx := chi.NewRouteContext()
			for key, value := range tt.params {
				chiCtx.URLParams.Add(key, value)
			}

			ctx := context.WithValue(context.Background(), chi.RouteCtxKey, chiCtx)
			logCtx := NewRequestLogger(logger, "GET", time.Now())

			// Write the log
			logCtx.WriteLog(ctx, http.StatusOK)

			require.Len(t, sink.entries, 1, "expected exactly one log entry")

			// Convert fields to map for easier checking
			fieldsMap := make(map[string]any)
			for _, field := range sink.entries[0].Fields {
				fieldsMap[field.Name] = field.Value
			}

			// Verify expected fields are present
			for _, field := range tt.expectedFields {
				value, exists := fieldsMap[field]
				require.True(t, exists, "field %q should be present in log", field)
				require.Equal(t, tt.params[strings.TrimPrefix(field, "params_")], value, "field %q has incorrect value", field)
			}

			// Verify no unexpected fields are present
			for field := range fieldsMap {
				if field == "took" || field == "status_code" || field == "latency_ms" {
					continue // Skip standard fields
				}
				require.True(t, slices.Contains(tt.expectedFields, field), "unexpected field %q in log", field)
			}
		})
	}
}

type fakeSink struct {
	entries    []slog.SinkEntry
	newEntries chan slog.SinkEntry
}

func (s *fakeSink) LogEntry(_ context.Context, e slog.SinkEntry) {
	s.entries = append(s.entries, e)
	if s.newEntries != nil {
		select {
		case s.newEntries <- e:
		default:
		}
	}
}

func (*fakeSink) Sync() {}
