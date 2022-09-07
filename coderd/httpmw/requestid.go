package httpmw

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"cdr.dev/slog"
)

type requestIDContextKey struct{}

// RequestID returns the ID of the request.
func RequestID(r *http.Request) uuid.UUID {
	rid, ok := r.Context().Value(requestIDContextKey{}).(uuid.UUID)
	if !ok {
		panic("developer error: request id middleware not provided")
	}
	return rid
}

// AttachRequestID adds a request ID to each HTTP request.
func AttachRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rid := uuid.New()

		ctx := context.WithValue(r.Context(), requestIDContextKey{}, rid)
		ctx = slog.With(ctx, slog.F("request_id", rid))

		rw.Header().Set("X-Coder-Request-Id", rid.String())
		next.ServeHTTP(rw, r.WithContext(ctx))
	})
}
