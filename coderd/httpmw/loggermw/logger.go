package loggermw

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/tracing"
)

var (
	safeParams  = []string{"page", "limit", "offset", "path"}
	countParams = []string{"ids", "template_ids"}
)

func safeQueryParams(params url.Values) []slog.Field {
	if len(params) == 0 {
		return nil
	}

	fields := make([]slog.Field, 0, len(params))
	for key, values := range params {
		// Check if this parameter should be included
		for _, pattern := range safeParams {
			if strings.EqualFold(key, pattern) {
				// Prepend query parameters in the log line to ensure we don't have issues with collisions
				// in case any other internal logging fields already log fields with similar names
				fieldName := "query_" + key

				// Log the actual values for non-sensitive parameters
				if len(values) == 1 {
					fields = append(fields, slog.F(fieldName, values[0]))
					continue
				}
				fields = append(fields, slog.F(fieldName, values))
			}
		}
		// Some query params we just want to log the count of the params length
		for _, pattern := range countParams {
			if !strings.EqualFold(key, pattern) {
				continue
			}
			count := 0

			// Prepend query parameters in the log line to ensure we don't have issues with collisions
			// in case any other internal logging fields already log fields with similar names
			fieldName := "query_" + key

			// Count comma-separated values for CSV format
			for _, v := range values {
				if strings.Contains(v, ",") {
					count += len(strings.Split(v, ","))
					continue
				}
				count++
			}
			// For logging we always want strings
			fields = append(fields, slog.F(fieldName+"_count", strconv.Itoa(count)))
		}
	}
	return fields
}

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

			// Add safe query parameters to the log
			if queryFields := safeQueryParams(r.URL.Query()); len(queryFields) > 0 {
				httplog = httplog.With(queryFields...)
			}

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

type SlogRequestLogger struct {
	log       slog.Logger
	written   bool
	message   string
	start     time.Time
	addFields func()
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

	if c.addFields != nil {
		c.addFields()
	}

	logger := c.log.With(
		slog.F("took", end.Sub(c.start)),
		slog.F("status_code", status),
		slog.F("latency_ms", float64(end.Sub(c.start)/time.Millisecond)),
	)

	// If the request is routed, add the route parameters to the log.
	if chiCtx := chi.RouteContext(ctx); chiCtx != nil {
		urlParams := chiCtx.URLParams
		routeParamsFields := make([]slog.Field, 0, len(urlParams.Keys))

		for k, v := range urlParams.Keys {
			if urlParams.Values[k] != "" {
				routeParamsFields = append(routeParamsFields, slog.F("params_"+v, urlParams.Values[k]))
			}
		}

		if len(routeParamsFields) > 0 {
			logger = logger.With(routeParamsFields...)
		}
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
