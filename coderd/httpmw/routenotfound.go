package httpmw

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
)

// RouteNotFound returns middleware that hides an entire route tree. Every
// request receives the standard route not found payload, so a route tree
// disabled by configuration is indistinguishable from one that was never
// registered.
func RouteNotFound() func(next http.Handler) http.Handler {
	return func(http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			httpapi.RouteNotFound(rw)
		})
	}
}
