package httpmw

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"cdr.dev/slog/v3"
)

type requestIDContextKey struct{}

// RequestID returns the ID of the request.
func RequestID(r *http.Request) uuid.UUID {
	rid, ok := RequestIDOptional(r)
	if !ok {
		panic("developer error: request id middleware not provided")
	}
	return rid
}

// RequestIDOptional returns the request ID when present.
func RequestIDOptional(r *http.Request) (uuid.UUID, bool) {
	rid, ok := r.Context().Value(requestIDContextKey{}).(uuid.UUID)
	return rid, ok
}

// WithRequestID stores a request ID in the context.
func WithRequestID(ctx context.Context, rid uuid.UUID) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, rid)
}

// AttachRequestID adds a request ID to each HTTP request.
func AttachRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rid := uuid.New()
		ridString := rid.String()

		ctx := context.WithValue(r.Context(), requestIDContextKey{}, rid)
		ctx = slog.With(ctx, slog.F("request_id", rid))

		trace.SpanFromContext(ctx).
			SetAttributes(attribute.String("request_id", rid.String()))

		rw.Header().Set("X-Coder-Request-Id", ridString)
		next.ServeHTTP(rw, r.WithContext(ctx))
	})
}
