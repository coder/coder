package tracing

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/coder/coder/coderd/httpapi"
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

			sw, ok := rw.(*httpapi.StatusWriter)
			if !ok {
				panic(fmt.Sprintf("ResponseWriter not a *httpapi.StatusWriter; got %T", rw))
			}

			// pass the span through the request context and serve the request to the next middleware
			next.ServeHTTP(sw, r)
			// capture response data
			EndHTTPSpan(r, sw.Status)
		})
	}
}

// EndHTTPSpan captures request and response data after the handler is done.
func EndHTTPSpan(r *http.Request, status int) {
	span := trace.SpanFromContext(r.Context())

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

	// 0 status means one has not yet been sent in which case net/http library will write StatusOK
	if status == 0 {
		status = http.StatusOK
	}
	span.SetAttributes(semconv.HTTPStatusCodeKey.Int(status))
	spanStatus, spanMessage := semconv.SpanStatusFromHTTPStatusCodeAndSpanKind(status, trace.SpanKindServer)
	span.SetStatus(spanStatus, spanMessage)

	// finally end span
	span.End()
}
