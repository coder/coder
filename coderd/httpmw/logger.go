package httpmw

import (
	"net/http"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/httpapi"
)

func Logger(log slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &httpapi.StatusWriter{ResponseWriter: w}

			httplog := log.With(
				slog.F("host", httpapi.RequestHost(r)),
				slog.F("path", r.URL.Path),
				slog.F("proto", r.Proto),
				slog.F("remote_addr", r.RemoteAddr),
			)

			next.ServeHTTP(sw, r)

			// Don't log successful health check requests.
			if r.URL.Path == "/api/v2" && sw.Status == 200 {
				return
			}

			httplog = httplog.With(
				slog.F("took", time.Since(start)),
				slog.F("status_code", sw.Status),
				slog.F("latency_ms", float64(time.Since(start)/time.Millisecond)),
			)

			// For status codes 400 and higher we
			// want to log the response body.
			if sw.Status >= 400 {
				httplog = httplog.With(
					slog.F("response_body", string(sw.ResponseBody())),
				)
			}

			// We should not log at level ERROR for 5xx status codes because 5xx
			// includes proxy errors etc. It also causes slogtest to fail
			// instantly without an error message by default.
			logLevelFn := httplog.Debug
			if sw.Status >= 400 {
				logLevelFn = httplog.Warn
			}

			logLevelFn(r.Context(), r.Method)
		})
	}
}
