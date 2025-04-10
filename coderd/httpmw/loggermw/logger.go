package loggermw

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/rbac"
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
	WithAuthContext(actor rbac.Subject)
}

type SlogRequestLogger struct {
	log     slog.Logger
	written bool
	message string
	start   time.Time
	mu      sync.RWMutex
	actors  map[rbac.SubjectType]rbac.Subject
}

var _ RequestLogger = &SlogRequestLogger{}

func NewRequestLogger(log slog.Logger, message string, start time.Time) RequestLogger {
	return &SlogRequestLogger{
		log:     log,
		written: false,
		message: message,
		start:   start,
		actors:  make(map[rbac.SubjectType]rbac.Subject),
	}
}

func (c *SlogRequestLogger) WithFields(fields ...slog.Field) {
	c.log = c.log.With(fields...)
}

func (c *SlogRequestLogger) WithAuthContext(actor rbac.Subject) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.actors[actor.Type] = actor
}

func (c *SlogRequestLogger) addAuthContextFields() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	usr, ok := c.actors[rbac.SubjectTypeUser]
	if ok {
		c.log = c.log.With(
			slog.F("requestor_id", usr.ID),
			slog.F("requestor_name", usr.FriendlyName),
			slog.F("requestor_email", usr.Email),
		)
	} else if len(c.actors) > 0 {
		for _, v := range c.actors {
			c.log = c.log.With(
				slog.F("requestor_name", v.FriendlyName),
			)
			break
		}
	}
}

func (c *SlogRequestLogger) WriteLog(ctx context.Context, status int) {
	if c.written {
		return
	}
	c.written = true
	end := time.Now()

	// Right before we write the log, we try to find the user in the actors
	// and add the fields to the log.
	c.addAuthContextFields()

	logger := c.log.With(
		slog.F("took", end.Sub(c.start)),
		slog.F("status_code", status),
		slog.F("latency_ms", float64(end.Sub(c.start)/time.Millisecond)),
	)

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
