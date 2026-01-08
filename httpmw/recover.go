package httpmw

import (
	"context"
	"net/http"
	"runtime/debug"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/tracing"
)

func Recover(log slog.Logger) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				r := recover()

				// Reverse proxying (among other things) may panic with
				// http.ErrAbortHandler when the request is aborted. It's not a
				// real panic so we shouldn't log them.
				//
				//nolint:errorlint // this is how the stdlib does the check
				if r != nil && r != http.ErrAbortHandler {
					log.Warn(context.Background(),
						"panic serving http request (recovered)",
						slog.F("panic", r),
						slog.F("stack", string(debug.Stack())),
					)

					var hijacked bool
					if sw, ok := w.(*tracing.StatusWriter); ok {
						hijacked = sw.Hijacked
					}

					// Only try to write errors on
					// non-hijacked responses.
					if !hijacked {
						httpapi.InternalServerError(w, nil)
					}
				}
			}()

			h.ServeHTTP(w, r)
		})
	}
}
