package httpmw

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/tracing"
)

func Logger(log slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			start := time.Now()

			sw, ok := rw.(*tracing.StatusWriter)
			if !ok {
				panic(fmt.Sprintf("ResponseWriter not a *tracing.StatusWriter; got %T", rw))
			}

			httplog := log.With(
				slog.F("host", httpapi.RequestHost(r)),
				slog.F("path", r.URL.Path),
				slog.F("proto", r.Proto),
				slog.F("remote_addr", r.RemoteAddr),
				// Include the start timestamp in the log so that we have the
				// source of truth. There is at least a theoretical chance that
				// there can be a delay between `next.ServeHTTP` ending and us
				// actually logging the request. This can also be useful when
				// filtering logs that started at a certain time (compared to
				// trying to compute the value).
				slog.F("start", start),
			)

			logContext := NewRequestLogger(httplog, r.Method, start)

			ctx := WithRequestLogger(r.Context(), logContext)

			next.ServeHTTP(sw, r.WithContext(ctx))

			// Don't log successful health check requests.
			if r.URL.Path == "/api/v2" && sw.Status == http.StatusOK {
				return
			}

			// For status codes 500 and higher we
			// want to log the response body.
			if sw.Status >= http.StatusInternalServerError {
				logContext.WithFields(
					slog.F("response_body", string(sw.ResponseBody())),
				)
			}

			logContext.WriteLog(r.Context(), sw.Status)
		})
	}
}

type RequestLogger interface {
	WithFields(fields ...slog.Field)
	WriteLog(ctx context.Context, status int)
}

type SlogRequestLogger struct {
	log     slog.Logger
	written bool
	message string
	start   time.Time
}

var _ RequestLogger = &SlogRequestLogger{}

func NewRequestLogger(log slog.Logger, message string, start time.Time) RequestLogger {
	return &SlogRequestLogger{
		log:     log,
		written: false,
		message: message,
		start:   start,
	}
}

func (c *SlogRequestLogger) WithFields(fields ...slog.Field) {
	c.log = c.log.With(fields...)
}

func (c *SlogRequestLogger) WriteLog(ctx context.Context, status int) {
	if c.written {
		return
	}
	c.written = true
	end := time.Now()

	logger := c.log.With(
		slog.F("took", end.Sub(c.start)),
		slog.F("status_code", status),
		slog.F("latency_ms", float64(end.Sub(c.start)/time.Millisecond)),
	)

	subject, ok := dbauthz.ActorFromContext(ctx)
	if ok {
		logger = c.log.With(
			slog.F("requestor_id", subject.ID),
			slog.F("requestor_email", subject.Email),
		)
	}
	// We already capture most of this information in the span (minus
	// the response body which we don't want to capture anyways).
	tracing.RunWithoutSpan(ctx, func(ctx context.Context) {
		// We should not log at level ERROR for 5xx status codes because 5xx
		// includes proxy errors etc. It also causes slogtest to fail
		// instantly without an error message by default.
		if status >= http.StatusInternalServerError {
			logger.Warn(ctx, c.message)
		} else {
			logger.Debug(ctx, c.message)
		}
	})
}

type logContextKey struct{}

func WithRequestLogger(ctx context.Context, rl RequestLogger) context.Context {
	return context.WithValue(ctx, logContextKey{}, rl)
}

func RequestLoggerFromContext(ctx context.Context) RequestLogger {
	val := ctx.Value(logContextKey{})
	if logCtx, ok := val.(RequestLogger); ok {
		return logCtx
	}
	return nil
}
