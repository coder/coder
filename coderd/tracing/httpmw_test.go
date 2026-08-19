package tracing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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
	attrs []attribute.KeyValue
}

func (s *recordingSpan) SetAttributes(kv ...attribute.KeyValue) {
	s.attrs = append(s.attrs, kv...)
}

func (s *recordingSpan) attributes() []attribute.KeyValue {
	return s.attrs
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

		val, ok := fieldValue(fields, "client_session_id")
		require.True(t, ok, "client_session_id should be on the log context")
		require.Equal(t, testSessionID, val)

		require.Contains(t, tp.span.attributes(), attribute.String("client_session_id", testSessionID))
	})

	t.Run("TracingEnabledNoBaggage", func(t *testing.T) {
		t.Parallel()

		// With tracing on but no baggage, the session ID is empty and the
		// middleware must not set an empty client_session_id span attribute or log
		// field.
		tp := &fakeTracer{span: &recordingSpan{Span: tracing.NoopSpan}}
		fields := requestFields(t, tp, "/api/v2/workspaces", "")

		_, ok := fieldValue(fields, "client_session_id")
		require.False(t, ok, "client_session_id should be absent when no baggage is sent")
		require.False(t, hasAttrKey(tp.span.attributes(), "client_session_id"),
			"no client_session_id attribute should be set when no baggage is sent")
	})

	t.Run("TracingDisabled", func(t *testing.T) {
		t.Parallel()

		// A nil tracer provider disables span creation, but the client_session_id
		// must still land on the log context.
		fields := requestFields(t, nil, "/api/v2/workspaces", tracing.SessionIDBaggageKey+"="+testSessionID)

		val, ok := fieldValue(fields, "client_session_id")
		require.True(t, ok, "client_session_id should be on the log context even when tracing is disabled")
		require.Equal(t, testSessionID, val)
	})

	t.Run("NoBaggage", func(t *testing.T) {
		t.Parallel()

		fields := requestFields(t, nil, "/api/v2/workspaces", "")
		_, ok := fieldValue(fields, "client_session_id")
		require.False(t, ok, "client_session_id should be absent when no baggage is sent")
	})

	t.Run("MalformedSessionID", func(t *testing.T) {
		t.Parallel()

		tp := &fakeTracer{span: &recordingSpan{Span: tracing.NoopSpan}}
		fields := requestFields(t, tp, "/api/v2/workspaces", tracing.SessionIDBaggageKey+"=not-a-valid-session-id")

		_, ok := fieldValue(fields, "client_session_id")
		require.False(t, ok, "malformed client_session_id should be ignored")
		require.False(t, hasAttrKey(tp.span.attributes(), "client_session_id"),
			"no client_session_id attribute should be set for a malformed session ID")
	})

	t.Run("QueryParameter", func(t *testing.T) {
		t.Parallel()

		// Browser WebSocket clients (such as the web terminal PTY) cannot set
		// baggage headers, so the middleware falls back to the
		// client_session_id query parameter.
		tp := &fakeTracer{span: &recordingSpan{Span: tracing.NoopSpan}}
		fields := requestFields(t, tp, "/api/v2/workspaces?"+tracing.SessionIDBaggageKey+"="+testSessionID, "")

		val, ok := fieldValue(fields, "client_session_id")
		require.True(t, ok, "client_session_id from the query parameter should be on the log context")
		require.Equal(t, testSessionID, val)
		require.Contains(t, tp.span.attributes(), attribute.String("client_session_id", testSessionID))
	})

	t.Run("BaggageTakesPrecedence", func(t *testing.T) {
		t.Parallel()

		// When both baggage and the query parameter are present, baggage wins.
		const querySessionID = "fedcba9876543210fedcba9876543210"
		tp := &fakeTracer{span: &recordingSpan{Span: tracing.NoopSpan}}
		fields := requestFields(t, tp,
			"/api/v2/workspaces?"+tracing.SessionIDBaggageKey+"="+querySessionID,
			tracing.SessionIDBaggageKey+"="+testSessionID)

		val, ok := fieldValue(fields, "client_session_id")
		require.True(t, ok, "client_session_id should be on the log context")
		require.Equal(t, testSessionID, val, "baggage should take precedence over the query parameter")
		require.Contains(t, tp.span.attributes(), attribute.String("client_session_id", testSessionID))
	})

	t.Run("MalformedQuerySessionID", func(t *testing.T) {
		t.Parallel()

		tp := &fakeTracer{span: &recordingSpan{Span: tracing.NoopSpan}}
		fields := requestFields(t, tp, "/api/v2/workspaces?"+tracing.SessionIDBaggageKey+"=not-a-valid-session-id", "")

		_, ok := fieldValue(fields, "client_session_id")
		require.False(t, ok, "malformed client_session_id query parameter should be ignored")
		require.False(t, hasAttrKey(tp.span.attributes(), "client_session_id"),
			"no client_session_id attribute should be set for a malformed query session ID")
	})

	t.Run("NonMatchingRoute", func(t *testing.T) {
		t.Parallel()

		// The middleware only runs on matched API/app routes. Static and
		// asset routes must not extract client_session_id, even from well-formed
		// baggage or a well-formed query parameter, so client-controlled
		// values are never logged for every request.
		tp := &fakeTracer{span: &recordingSpan{Span: tracing.NoopSpan}}
		fields := requestFields(t, tp, "/index.html?"+tracing.SessionIDBaggageKey+"="+testSessionID, tracing.SessionIDBaggageKey+"="+testSessionID)

		_, ok := fieldValue(fields, "client_session_id")
		require.False(t, ok, "client_session_id must not be logged on a non-matching route")
		require.False(t, hasAttrKey(tp.span.attributes(), "client_session_id"),
			"no client_session_id attribute should be set on a non-matching route")
	})

	// FieldNamesMatchBaggageKey pins the baggage key, the log field name, and
	// the span attribute name to the same value. slog field names must be
	// snake_case string literals, so the log field and span attribute cannot
	// reference SessionIDBaggageKey directly; this test guards against the
	// three drifting apart and silently breaking log/trace correlation.
	t.Run("FieldNamesMatchBaggageKey", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "client_session_id", tracing.SessionIDBaggageKey)

		tp := &fakeTracer{span: &recordingSpan{Span: tracing.NoopSpan}}
		fields := requestFields(t, tp, "/api/v2/workspaces", tracing.SessionIDBaggageKey+"="+testSessionID)

		_, ok := fieldValue(fields, tracing.SessionIDBaggageKey)
		require.True(t, ok, "log field name must match the baggage key")
		require.Contains(t, tp.span.attributes(),
			attribute.String(tracing.SessionIDBaggageKey, testSessionID),
			"span attribute name must match the baggage key")
	})

	// QuerySessionIDNotInSpanName guards the interaction between the
	// query-parameter fallback and span naming. A client_session_id supplied
	// via the query string (as the web terminal PTY does) must surface as the
	// client_session_id span attribute but must never leak into an exported
	// span name, which is emitted to tracing backends at span start.
	t.Run("QuerySessionIDNotInSpanName", func(t *testing.T) {
		t.Parallel()

		startNames := &startNameRecorder{}
		recorder := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(
			sdktrace.WithSpanProcessor(startNames),
			sdktrace.WithSpanProcessor(recorder),
		)

		rw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}
		r := httptest.NewRequest(http.MethodGet,
			"/api/v2/workspaceagents/abc/pty?"+tracing.SessionIDBaggageKey+"="+testSessionID, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		ctx = context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext())
		r = r.WithContext(ctx)

		tracing.Middleware(provider)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})).ServeHTTP(rw, r)

		require.NoError(t, provider.ForceFlush(ctx))

		// Neither the span-start name nor the exported span name may contain
		// the session ID, since the initial name is built from the path only.
		require.NotEmpty(t, startNames.names)
		for _, name := range startNames.names {
			require.NotContains(t, name, testSessionID,
				"span start name must not carry the query session ID")
		}
		spans := recorder.Ended()
		require.NotEmpty(t, spans)
		for _, span := range spans {
			require.NotContains(t, span.Name(), testSessionID,
				"exported span name must not carry the query session ID")
		}

		// The ID must still be recorded as the dedicated span attribute.
		var found bool
		for _, span := range spans {
			for _, attr := range span.Attributes() {
				if string(attr.Key) == tracing.SessionIDBaggageKey {
					require.Equal(t, testSessionID, attr.Value.AsString())
					found = true
				}
			}
		}
		require.True(t, found,
			"client_session_id from the query parameter must be recorded as a span attribute")
	})
}

// startNameRecorder captures span names as they are at span start, before
// EndHTTPSpan renames them, because span names are exported to tracing
// backends at span start.
type startNameRecorder struct {
	mu    sync.Mutex
	names []string
}

func (r *startNameRecorder) OnStart(_ context.Context, s sdktrace.ReadWriteSpan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = append(r.names, s.Name())
}

func (*startNameRecorder) OnEnd(sdktrace.ReadOnlySpan)      {}
func (*startNameRecorder) Shutdown(context.Context) error   { return nil }
func (*startNameRecorder) ForceFlush(context.Context) error { return nil }

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

	t.Run("QueryCredentialsNotExported", func(t *testing.T) {
		t.Parallel()

		// Some endpoints accept bearer credentials as query parameters
		// (e.g. signed chat file download tokens). No exported span name
		// or attribute may carry the query string.
		const token = "super-secret-download-token"

		startNames := &startNameRecorder{}
		recorder := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(
			sdktrace.WithSpanProcessor(startNames),
			sdktrace.WithSpanProcessor(recorder),
		)

		rw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}
		r := httptest.NewRequest("GET", "/api/experimental/chats/files/abc/download?token="+token, nil)

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
		defer cancel()
		ctx = context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext())
		r = r.WithContext(ctx)

		tracing.Middleware(provider)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})).ServeHTTP(rw, r)

		require.NoError(t, provider.ForceFlush(ctx))
		require.NotEmpty(t, startNames.names)
		for _, name := range startNames.names {
			require.NotContains(t, name, token)
		}
		spans := recorder.Ended()
		require.NotEmpty(t, spans)
		for _, span := range spans {
			require.NotContains(t, span.Name(), token)
			for _, attr := range span.Attributes() {
				require.NotContains(t, attr.Value.Emit(), token,
					"span attribute %s must not carry query credentials", attr.Key)
			}
		}
	})
}
