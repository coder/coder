package tracing_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/testutil"
)

type fakeTracerProvider struct {
	noop.TracerProvider
	startCalled int64
}

type fakeTracer struct {
	noop.Tracer
	prov *fakeTracerProvider
}

var (
	_ trace.TracerProvider = &fakeTracerProvider{}
	_ trace.Tracer         = &fakeTracer{}
)

func (f *fakeTracer) Start(ctx context.Context, str string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return f.prov.Start(ctx, str, opts...)
}

// Tracer implements trace.TracerProvider.
func (f *fakeTracerProvider) Tracer(_ string, _ ...trace.TracerOption) trace.Tracer {
	return &fakeTracer{prov: f}
}

// Start implements trace.Tracer.
func (f *fakeTracerProvider) Start(ctx context.Context, _ string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	atomic.AddInt64(&f.startCalled, 1)
	return ctx, tracing.NoopSpan
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
			c := c

			name := strings.ReplaceAll(strings.TrimPrefix(c.path, "/"), "/", "_")
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				fake := &fakeTracerProvider{}

				rw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}
				r := httptest.NewRequest("GET", c.path, nil)

				ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
				defer cancel()
				ctx = context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext())
				r = r.WithContext(ctx)

				tracing.Middleware(fake)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
					rw.WriteHeader(http.StatusNoContent)
				})).ServeHTTP(rw, r)

				didRun := atomic.LoadInt64(&fake.startCalled) == 1
				require.Equal(t, c.runs, didRun, "expected middleware to run/not run")
			})
		}
	})
}
