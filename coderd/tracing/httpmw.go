package tracing

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	semconv "go.opentelemetry.io/otel/semconv/v1.11.0"
	"go.opentelemetry.io/otel/trace"
)

// Middleware adds tracing to http routes.
func Middleware(tracerProvider trace.TracerProvider) func(http.Handler) http.Handler {
	var tracer trace.Tracer
	if tracerProvider != nil {
		tracer = tracerProvider.Tracer(TracerName)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if tracer == nil {
				next.ServeHTTP(rw, r)
				return
			}

			// start span with default span name. Span name will be updated to "method route" format once request finishes.
			ctx, span := tracer.Start(r.Context(), fmt.Sprintf("%s %s", r.Method, r.RequestURI))
			defer span.End()
			r = r.WithContext(ctx)

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
	span.SetAttributes(semconv.NetAttributesFromHTTPRequest("tcp", r)...)
	span.SetAttributes(semconv.EndUserAttributesFromHTTPRequest(r)...)
	span.SetAttributes(semconv.HTTPServerAttributesFromHTTPRequest("", route, r)...)
	span.SetAttributes(semconv.HTTPRouteKey.String(route))

	// 0 status means one has not yet been sent in which case net/http library will write StatusOK
	if status == 0 {
		status = http.StatusOK
	}
	span.SetAttributes(semconv.HTTPStatusCodeKey.Int(status))
	span.SetStatus(semconv.SpanStatusFromHTTPStatusCodeAndSpanKind(status, trace.SpanKindServer))

	// finally end span
	span.End()
}

func StartSpan(ctx context.Context, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return trace.SpanFromContext(ctx).TracerProvider().Tracer(TracerName).Start(ctx, FuncNameSkip(1), opts...)
}
