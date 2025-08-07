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
		pprof.Do(ctx, pproflabel.Service(pproflabel.ServiceHTTPServer), func(ctx context.Context) {
			r = r.WithContext(ctx)
			next.ServeHTTP(rw, r)
		})
	})
}
