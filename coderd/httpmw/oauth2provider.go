package httpmw

import (
	"net/http"

	"github.com/coder/coder/v2/coderd/httpapi"
)

// RequireOAuth2Provider returns 404 for every request while the OAuth2
// provider is disabled. A disabled provider looks the same as a deployment
// that never had one, which is what RFC 8414 and RFC 9728 discovery clients
// expect. There is no bypass for development builds.
//
// enabled is a func so the check can become a runtime setting later without
// changing call sites.
func RequireOAuth2Provider(enabled func() bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if !enabled() {
				httpapi.RouteNotFound(rw)
				return
			}
			next.ServeHTTP(rw, r)
		})
	}
}
