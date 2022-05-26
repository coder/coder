package httpmw

import (
	"net/http"
	"strings"
)

// Wildcard routes to the provided handler if the request host has the suffix of hostname.
func Wildcard(hostname string, handler http.HandlerFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var (
				ctx = r.Context()
			)

			if !strings.HasSuffix(r.Host, hostname) {
				next.ServeHTTP(w, r)
				return
			}

			handler(w, r.WithContext(ctx))
		})
	}
}
