package tracing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/testutil"
)

// noopTracer is just an alias because the fakeTracer implements a method
// with the same name 'Tracer'. Kinda dumb, but this is a workaround.
type noopTracer = noop.Tracer

type fakeTracer struct {
	noop.TracerProvider
	noopTracer
	startCalled atomic.Int64
	// span, when set, is returned from Start so tests can assert on the
	// attributes the middleware records. When nil, Start returns
	// tracing.NoopSpan.
	span *recordingSpan
}

var (
	_ trace.TracerProvider = &fakeTracer{}
	_ trace.Tracer         = &fakeTracer{}
)

// Tracer implements trace.TracerProvider.
func (f *fakeTracer) Tracer(_ string, _ ...trace.TracerOption) trace.Tracer {
	return f
}

// Start implements trace.Tracer.
func (f *fakeTracer) Start(ctx context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	f.startCalled.Add(1)
	if f.span != nil {
		return ctx, f.span
	}
	return ctx, tracing.NoopSpan
}

// recordingSpan wraps a noop span and records the attributes set on it so
// tests can assert on span attributes.
type recordingSpan struct {
	trace.Span
	mu    sync.Mutex
	attrs []attribute.KeyValue
}

func (s *recordingSpan) SetAttributes(kv ...attribute.KeyValue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attrs = append(s.attrs, kv...)
}

func (s *recordingSpan) attributes() []attribute.KeyValue {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.attrs)
}

const testSessionID = "0123456789abcdef0123456789abcdef"

func Test_Middleware_SessionID(t *testing.T) {
	t.Parallel()

	// requestFields serves a request through the middleware and returns the
	// fields logged by a downstream handler using the request context.
	requestFields := func(t *testing.T, tp trace.TracerProvider, path, header string) []slog.Field {
		t.Helper()

		sink := testutil.NewFakeSink(t)
		logger := sink.Logger()

		handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Logging with the request context surfaces any fields the
			// middleware added via slog.With.
			logger.Info(r.Context(), "downstream handler invoked")
			rw.WriteHeader(http.StatusNoContent)
		})

		rw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if header != "" {
			r.Header.Set("baggage", header)
		}

		ctx := context.WithValue(context.Background(), chi.RouteCtxKey, chi.NewRouteContext())
		r = r.WithContext(ctx)

		tracing.Middleware(tp)(handler).ServeHTTP(rw, r)

		entries := sink.Entries(func(e slog.SinkEntry) bool {
			return e.Message == "downstream handler invoked"
		})
		require.Len(t, entries, 1)
		return entries[0].Fields
	}

	fieldValue := func(fields []slog.Field, name string) (any, bool) {
		for _, f := range fields {
			if f.Name == name {
				return f.Value, true
			}
		}
		return nil, false
	}

	hasAttrKey := func(attrs []attribute.KeyValue, key string) bool {
		for _, a := range attrs {
			if string(a.Key) == key {
				return true
			}
		}
		return false
	}

	t.Run("TracingEnabled", func(t *testing.T) {
		t.Parallel()

		tp := &fakeTracer{span: &recordingSpan{Span: tracing.NoopSpan}}
		fields := requestFields(t, tp, "/api/v2/workspaces", tracing.SessionIDBaggageKey+"="+testSessionID)

		val, ok := fieldValue(fields, "session_id")
		require.True(t, ok, "session_id should be on the log context")
		require.Equal(t, testSessionID, val)

		require.Contains(t, tp.span.attributes(), attribute.String("session_id", testSessionID))
	})

	t.Run("TracingEnabledNoBaggage", func(t *testing.T) {
		t.Parallel()

		// With tracing on but no baggage, the session ID is empty and the
		// middleware must not set an empty session_id span attribute or log
		// field.
		tp := &fakeTracer{span: &recordingSpan{Span: tracing.NoopSpan}}
		fields := requestFields(t, tp, "/api/v2/workspaces", "")

		_, ok := fieldValue(fields, "session_id")
		require.False(t, ok, "session_id should be absent when no baggage is sent")
		require.False(t, hasAttrKey(tp.span.attributes(), "session_id"),
			"no session_id attribute should be set when no baggage is sent")
	})

	t.Run("TracingDisabled", func(t *testing.T) {
		t.Parallel()

		// A nil tracer provider disables span creation, but the session_id
		// must still land on the log context.
		fields := requestFields(t, nil, "/api/v2/workspaces", tracing.SessionIDBaggageKey+"="+testSessionID)

		val, ok := fieldValue(fields, "session_id")
		require.True(t, ok, "session_id should be on the log context even when tracing is disabled")
		require.Equal(t, testSessionID, val)
	})

	t.Run("NoBaggage", func(t *testing.T) {
		t.Parallel()

		fields := requestFields(t, nil, "/api/v2/workspaces", "")
		_, ok := fieldValue(fields, "session_id")
		require.False(t, ok, "session_id should be absent when no baggage is sent")
	})

	t.Run("MalformedBaggage", func(t *testing.T) {
		t.Parallel()

		tp := &fakeTracer{span: &recordingSpan{Span: tracing.NoopSpan}}
		fields := requestFields(t, tp, "/api/v2/workspaces", tracing.SessionIDBaggageKey+"=not-a-valid-session-id")

		_, ok := fieldValue(fields, "session_id")
		require.False(t, ok, "malformed session_id should be ignored")
		require.False(t, hasAttrKey(tp.span.attributes(), "session_id"),
			"no session_id attribute should be set for malformed baggage")
	})

	t.Run("NonMatchingRoute", func(t *testing.T) {
		t.Parallel()

		// The middleware only runs on matched API/app routes. Static and
		// asset routes must not extract session_id, even from well-formed
		// baggage, so client-controlled baggage is never logged for every
		// request.
		tp := &fakeTracer{span: &recordingSpan{Span: tracing.NoopSpan}}
		fields := requestFields(t, tp, "/index.html", tracing.SessionIDBaggageKey+"="+testSessionID)

		_, ok := fieldValue(fields, "session_id")
		require.False(t, ok, "session_id must not be logged on a non-matching route")
		require.False(t, hasAttrKey(tp.span.attributes(), "session_id"),
			"no session_id attribute should be set on a non-matching route")
	})

	// FieldNamesMatchBaggageKey pins the baggage key, the log field name, and
	// the span attribute name to the same value. slog field names must be
	// snake_case string literals, so the log field and span attribute cannot
	// reference SessionIDBaggageKey directly; this test guards against the
	// three drifting apart and silently breaking log/trace correlation.
	t.Run("FieldNamesMatchBaggageKey", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "session_id", tracing.SessionIDBaggageKey)

		tp := &fakeTracer{span: &recordingSpan{Span: tracing.NoopSpan}}
		fields := requestFields(t, tp, "/api/v2/workspaces", tracing.SessionIDBaggageKey+"="+testSessionID)

		_, ok := fieldValue(fields, tracing.SessionIDBaggageKey)
		require.True(t, ok, "log field name must match the baggage key")
		require.Contains(t, tp.span.attributes(),
			attribute.String(tracing.SessionIDBaggageKey, testSessionID),
			"span attribute name must match the baggage key")
	})
}

func Test_Middleware(t *testing.T) {
	t.Parallel()

	t.Run("OnlyRunsOnExpectedRoutes", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			path string
			runs bool
		}{
			// Should pass.
			{"/api", true},
			{"/api/v0", true},
			{"/api/v2", true},
			{"/api/v2/workspaces/", true},
			{"/api/v2/workspaces", true},
			{"/@hi/hi/apps/hi", true},
			{"/@hi/hi/apps/hi/hi", true},
			{"/@hi/hi/apps/hi/hi", true},
			{"/%40hi/hi/apps/hi", true},
			{"/%40hi/hi/apps/hi/hi", true},
			{"/%40hi/hi/apps/hi/hi", true},
			{"/external-auth/hi/callback", true},

			// Other routes that should not be collected.
			{"/index.html", false},
			{"/static/coder_linux_amd64", false},
			{"/workspaces", false},
			{"/templates", false},
			{"/@hi/hi/terminal", false},
		}

		for _, c := range cases {
			name := strings.ReplaceAll(strings.TrimPrefix(c.path, "/"), "/", "_")
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				fake := &fakeTracer{}

				rw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}
				r := httptest.NewRequest("GET", c.path, nil)

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
				defer cancel()
				ctx = context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext())
				r = r.WithContext(ctx)

				tracing.Middleware(fake)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
					rw.WriteHeader(http.StatusNoContent)
				})).ServeHTTP(rw, r)

				didRun := fake.startCalled.Load() == 1
				require.Equal(t, c.runs, didRun, "expected middleware to run/not run")
			})
		}
	})
}
