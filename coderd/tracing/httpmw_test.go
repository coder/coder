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
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

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
	return ctx, tracing.NoopSpan
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
