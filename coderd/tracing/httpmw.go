package tracing

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
)

// HTTPMW adds tracing to http routes.
func HTTPMW(tracerProvider *sdktrace.TracerProvider, name string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if tracerProvider == nil {
				next.ServeHTTP(rw, r)
				return
			}

			// start span with default span name. Span name will be updated to "method route" format once request finishes.
			ctx, span := tracerProvider.Tracer(name).Start(r.Context(), fmt.Sprintf("%s %s", r.Method, r.RequestURI))
			defer span.End()
			r = r.WithContext(ctx)

			wrw := middleware.NewWrapResponseWriter(rw, r.ProtoMajor)

			// pass the span through the request context and serve the request to the next middleware
			next.ServeHTTP(rw, r)

			// set the resource name as we get it only once the handler is executed
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route != "" {
				span.SetName(fmt.Sprintf("%s %s", r.Method, route))
			}
			span.SetName(fmt.Sprintf("%s %s", r.Method, route))
			span.SetAttributes(semconv.NetAttributesFromHTTPRequest("tcp", r)...)
			span.SetAttributes(semconv.EndUserAttributesFromHTTPRequest(r)...)
			span.SetAttributes(semconv.HTTPServerAttributesFromHTTPRequest("", route, r)...)
			span.SetAttributes(semconv.HTTPRouteKey.String(route))

			// set the status code
			status := wrw.Status()
			// 0 status means one has not yet been sent in which case net/http library will write StatusOK
			if status == 0 {
				status = http.StatusOK
			}
			span.SetAttributes(semconv.HTTPStatusCodeKey.Int(status))
			spanStatus, spanMessage := semconv.SpanStatusFromHTTPStatusCode(status)
			span.SetStatus(spanStatus, spanMessage)
		})
	}
}
