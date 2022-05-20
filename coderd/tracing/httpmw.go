package tracing

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// HTTPMW adds tracing to http routes.
func HTTPMW(tracerProvider *sdktrace.TracerProvider, name string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// do not trace if exporter has not be initialized
			if tracerProvider == nil {
				next.ServeHTTP(rw, r)
				return
			}

			// start span with default span name. Span name will be updated to "method route" format once request finishes.
			_, span := tracerProvider.Tracer(name).Start(r.Context(), fmt.Sprintf("%s %s", r.Method, r.RequestURI))
			defer span.End()

			wrw := middleware.NewWrapResponseWriter(rw, r.ProtoMajor)

			// pass the span through the request context and serve the request to the next middleware
			next.ServeHTTP(rw, r)

			// set the resource name as we get it only once the handler is executed
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route != "" {
				span.SetName(fmt.Sprintf("%s %s", r.Method, route))
			}
			span.SetAttributes(attribute.KeyValue{
				Key:   "http.method",
				Value: attribute.StringValue(r.Method),
			})
			span.SetAttributes(attribute.KeyValue{
				Key:   "http.route",
				Value: attribute.StringValue(route),
			})
			span.SetAttributes(attribute.KeyValue{
				Key:   "http.path",
				Value: attribute.StringValue(r.URL.EscapedPath()),
			})

			// set the status code
			status := wrw.Status()
			// 0 status means one has not yet been sent in which case net/http library will write StatusOK
			if status == 0 {
				status = http.StatusOK
			}
			span.SetAttributes(attribute.KeyValue{
				Key:   "http.status_code",
				Value: attribute.IntValue(status),
			})

			// if 5XX we set the span to "error" status
			if status >= 500 {
				span.SetStatus(codes.Error, fmt.Sprintf("%d: %s", status, http.StatusText(status)))
			}
		})
	}
}
