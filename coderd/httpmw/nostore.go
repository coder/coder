package httpmw

import "net/http"

const (
	cacheControlHeader = "Cache-Control"
	pragmaHeader       = "Pragma"
)

// NoStore sets the response caching headers that OAuth2 requires on any
// response that may contain a credential. RFC 6749 §5.1 makes both headers a
// MUST for the authorization server; OAuth 2.1 §3.2.3 keeps only no-store,
// because RFC 9111 §5.4 deprecates Pragma as a request-only field. Both are
// sent so that a client or auditor reading either specification sees a
// conformant response.
//
// The headers are set before the wrapped handler runs, so a handler that
// writes its own Cache-Control would win. No handler under /oauth2 does; the
// test suite pins that.
//
// NoStore carries no configuration, so it is the middleware itself rather
// than a constructor for one. Pass it to chi's r.Use without calling it.
func NoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set(cacheControlHeader, "no-store")
		rw.Header().Set(pragmaHeader, "no-cache")
		next.ServeHTTP(rw, r)
	})
}
