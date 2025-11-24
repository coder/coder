package httpmw

import (
	"context"
	"net/http"
	"runtime/pprof"

	"github.com/coder/coder/v2/coderd/pproflabel"
)

// WithProfilingLabels adds a pprof label to all http request handlers. This is
// primarily used to determine if load is coming from background jobs, or from
// http traffic.
func WithProfilingLabels(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Label to differentiate between http and websocket requests. Websocket requests
		// are assumed to be long-lived and more resource consuming.
		requestType := "http"
		if r.Header.Get("Upgrade") == "websocket" {
			requestType = "websocket"
		}

		pprof.Do(ctx, pproflabel.Service(pproflabel.ServiceHTTPServer, pproflabel.RequestTypeTag, requestType), func(ctx context.Context) {
			r = r.WithContext(ctx)
			next.ServeHTTP(rw, r)
		})
	})
}

func WithStaticProfilingLabels(labels pprof.LabelSet) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			pprof.Do(ctx, labels, func(ctx context.Context) {
				r = r.WithContext(ctx)
				next.ServeHTTP(rw, r)
			})
		})
	}
}
