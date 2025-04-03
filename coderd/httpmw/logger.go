package httpmw

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cdr.dev/slog"
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

			logContext := NewRequestLoggerContext(httplog, r.Method, start)

			ctx := context.WithValue(r.Context(), logContextKey{}, logContext)

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

			// We already capture most of this information in the span (minus
			// the response body which we don't want to capture anyways).
			tracing.RunWithoutSpan(r.Context(), func(ctx context.Context) {
				// logLevelFn(ctx, r.Method)
				logContext.WriteLog(r.Context(), sw.Status)
			})
		})
	}
}

type RequestLoggerContext struct {
	log     slog.Logger
	written bool
	message string
	start   time.Time
}

func NewRequestLoggerContext(log slog.Logger, message string, start time.Time) *RequestLoggerContext {
	return &RequestLoggerContext{
		log:     log,
		written: false,
		message: message,
		start:   start,
	}
}

func (c *RequestLoggerContext) WithFields(fields ...slog.Field) {
	c.log = c.log.With(fields...)
}

func (c *RequestLoggerContext) WriteLog(ctx context.Context, status int) {
	if c.written {
		return
	}
	c.written = true
	end := time.Now()

	c.WithFields(
		slog.F("took", end.Sub(c.start)),
		slog.F("status_code", status),
		slog.F("latency_ms", float64(end.Sub(c.start)/time.Millisecond)),
	)
	// We should not log at level ERROR for 5xx status codes because 5xx
	// includes proxy errors etc. It also causes slogtest to fail
	// instantly without an error message by default.
	if status >= http.StatusInternalServerError {
		c.log.Error(ctx, c.message, "status_code", status)
	} else {
		c.log.Debug(ctx, c.message, "status_code", status)
	}
}

type logContextKey struct{}

func RequestLoggerFromContext(ctx context.Context) *RequestLoggerContext {
	val := ctx.Value(logContextKey{})
	if logCtx, ok := val.(*RequestLoggerContext); ok {
		return logCtx
	}
	return nil
}
