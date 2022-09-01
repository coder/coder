package httpmw

import (
	"context"
	"net/http"
	"runtime/debug"

	"cdr.dev/slog"
	"github.com/coder/coder/coderd/httpapi"
)

func Recover(log slog.Logger) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				r := recover()
				if r != nil {
					log.Warn(context.Background(),
						"panic serving http request (recovered)",
						slog.F("panic", r),
						slog.F("stack", string(debug.Stack())),
					)

					var hijacked bool
					if sw, ok := w.(*httpapi.StatusWriter); ok {
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
