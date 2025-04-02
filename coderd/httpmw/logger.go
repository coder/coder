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

			logContext := &RequestLoggerContext{
				Fields:  map[string]any{},
				log:     &httplog,
				written: false,
			}
			defer func() {
				logContext.WriteLog(r.Context(), "", sw.Status)
			}()

			ctx := context.WithValue(r.Context(), logContextKey{}, logContext)

			next.ServeHTTP(sw, r.WithContext(ctx))

			end := time.Now()

			// Don't log successful health check requests.
			if r.URL.Path == "/api/v2" && sw.Status == http.StatusOK {
				return
			}

			httplog = httplog.With(
				slog.F("took", end.Sub(start)),
				slog.F("status_code", sw.Status),
				slog.F("latency_ms", float64(end.Sub(start)/time.Millisecond)),
			)

			// For status codes 400 and higher we
			// want to log the response body.
			if sw.Status >= http.StatusInternalServerError {
				httplog = httplog.With(
					slog.F("response_body", string(sw.ResponseBody())),
				)
			}

			// We should not log at level ERROR for 5xx status codes because 5xx
			// includes proxy errors etc. It also causes slogtest to fail
			// instantly without an error message by default.
			logLevelFn := httplog.Debug
			if sw.Status >= http.StatusInternalServerError {
				logLevelFn = httplog.Warn
			}

			// We already capture most of this information in the span (minus
			// the response body which we don't want to capture anyways).
			tracing.RunWithoutSpan(r.Context(), func(ctx context.Context) {
				logLevelFn(ctx, r.Method)
			})
		})
	}
}

type RequestLoggerContext struct {
	Fields map[string]any

	log     *slog.Logger
	written bool
}

func (c *RequestLoggerContext) WriteLog(ctx context.Context, msg string, status int) {
	if c.written {
		return
	}
	c.written = true
	// append extra fields to the logger
	for k, v := range c.Fields {
		c.log.With(slog.F(k, v))
	}

	if status >= http.StatusInternalServerError {
		c.log.Error(ctx, msg)
	} else {
		c.log.Debug(ctx, msg)
	}
}

type logContextKey struct{}

func FromContext(ctx context.Context) *RequestLoggerContext {
	val := ctx.Value(logContextKey{})
	if logCtx, ok := val.(*RequestLoggerContext); ok {
		return logCtx
	}
	return nil
}
