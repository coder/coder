package httpapi

import (
	"context"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpmw/loggermw"
)

// RequestBodyLimitTracker records that a request was rejected for exceeding a
// request body size limit.
//
// Middleware installs it on the way in and reads it on the way out, so a
// rejection written deep in a handler can be attributed without that handler
// knowing a metric exists. It is written and read on the request's own
// goroutine, so it needs no synchronization.
type RequestBodyLimitTracker struct {
	exceeded bool
}

// Exceeded reports whether a body size limit rejected this request.
func (t *RequestBodyLimitTracker) Exceeded() bool {
	return t.exceeded
}

type requestBodyLimitContextKey struct{}

// WithRequestBodyLimitTracker returns a context carrying tracker.
func WithRequestBodyLimitTracker(ctx context.Context, tracker *RequestBodyLimitTracker) context.Context {
	return context.WithValue(ctx, requestBodyLimitContextKey{}, tracker)
}

// RecordRequestBodyLimit reports the body size limit that rejected this
// request. It names the limit on the request's existing log line and marks the
// request so middleware can tell a body size rejection from the other reasons
// coderd answers 413, such as agent log storage overflow.
//
// The limit goes on the existing log line rather than one of its own: a caller
// can produce 413s at will, so a dedicated line is attacker-controlled log
// volume.
//
// Every site that answers 413 because a request body exceeded a limit must call
// this, and a site answering 413 for any other reason must not. ctx must be the
// request's context, which is what carries both the logger and the tracker.
func RecordRequestBodyLimit(ctx context.Context, limit int64) {
	if requestLogger := loggermw.RequestLoggerFromContext(ctx); requestLogger != nil {
		requestLogger.WithFields(slog.F("max_request_body_bytes", limit))
	}
	if tracker, ok := ctx.Value(requestBodyLimitContextKey{}).(*RequestBodyLimitTracker); ok {
		tracker.exceeded = true
	}
}
