package tracing

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.14.0"
	"go.opentelemetry.io/otel/semconv/v1.14.0/httpconv"
	"go.opentelemetry.io/otel/semconv/v1.14.0/netconv"
	"go.opentelemetry.io/otel/trace"

	"github.com/coder/coder/v2/coderd/httpmw/patternmatcher"
)

// Middleware adds tracing to http routes.
func Middleware(tracerProvider trace.TracerProvider) func(http.Handler) http.Handler {
	// We only want to create spans on the following route patterns, however
	// we want the middleware to be very high in the middleware stack so it can
	// capture the entire request.
	re := patternmatcher.RoutePatterns{
		"/api",
		"/api/**",
		"/@*/*/apps/**",
		"/%40*/*/apps/**",
		"/external-auth/*/callback",
	}.MustCompile()

	var tracer trace.Tracer
	if tracerProvider != nil {
		tracer = tracerProvider.Tracer(TracerName)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if tracer == nil || !re.MatchString(r.URL.Path) {
				next.ServeHTTP(rw, r)
				return
			}

			// Extract the trace context from the request headers.
			tmp := otel.GetTextMapPropagator()
			hc := propagation.HeaderCarrier(r.Header)
			ctx := tmp.Extract(r.Context(), hc)

			// start span with default span name. Span name will be updated to "method route" format once request finishes.
			ctx, span := tracer.Start(ctx, fmt.Sprintf("%s %s", r.Method, r.RequestURI))
			defer span.End()
			r = r.WithContext(ctx)

			if span.SpanContext().HasTraceID() && span.SpanContext().HasSpanID() {
				// Technically these values are included in the Traceparent
				// header, but they are easier to read for humans this way.
				rw.Header().Set("X-Trace-ID", span.SpanContext().TraceID().String())
				rw.Header().Set("X-Span-ID", span.SpanContext().SpanID().String())

				// Inject the trace context into the response headers.
				hc := propagation.HeaderCarrier(rw.Header())
				tmp.Inject(ctx, hc)
			}

			sw, ok := rw.(*StatusWriter)
			if !ok {
				panic(fmt.Sprintf("ResponseWriter not a *tracing.StatusWriter; got %T", rw))
			}

			// pass the span through the request context and serve the request to the next middleware
			next.ServeHTTP(sw, r)
			// capture response data
			EndHTTPSpan(r, sw.Status, span)
		})
	}
}

// EndHTTPSpan captures request and response data after the handler is done.
func EndHTTPSpan(r *http.Request, status int, span trace.Span) {
	// set the resource name as we get it only once the handler is executed
	route := chi.RouteContext(r.Context()).RoutePattern()
	span.SetName(fmt.Sprintf("%s %s", r.Method, route))
	span.SetAttributes(netconv.Transport("tcp"))
	span.SetAttributes(httpconv.ServerRequest("coderd", r)...)
	span.SetAttributes(semconv.HTTPRouteKey.String(route))

	// 0 status means one has not yet been sent in which case net/http library will write StatusOK
	if status == 0 {
		status = http.StatusOK
	}
	span.SetAttributes(semconv.HTTPStatusCodeKey.Int(status))
	span.SetStatus(httpconv.ServerStatus(status))

	// finally end span
	span.End()
}

type tracerNameKey struct{}

// SetTracerName sets the tracer name that will be used by all spans created
// from the context.
func SetTracerName(ctx context.Context, tracerName string) context.Context {
	return context.WithValue(ctx, tracerNameKey{}, tracerName)
}

// GetTracerName returns the tracer name from the context, or TracerName if none
// is set.
func GetTracerName(ctx context.Context) string {
	if tracerName, ok := ctx.Value(tracerNameKey{}).(string); ok {
		return tracerName
	}

	return TracerName
}

// StartSpan calls StartSpanWithName with the name set to the caller's function
// name.
func StartSpan(ctx context.Context, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return StartSpanWithName(ctx, FuncNameSkip(1), opts...)
}

// StartSpanWithName starts a new span with the given name from the context. If
// a tracer name was set on the context (or one of its parents), it will be used
// as the tracer name instead of the default TracerName.
func StartSpanWithName(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracerName := GetTracerName(ctx)
	return trace.SpanFromContext(ctx).
		TracerProvider().
		Tracer(tracerName).
		Start(ctx, name, opts...)
}
