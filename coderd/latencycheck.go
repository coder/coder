package coderd

import (
	"net/http"
	"strings"
)

func LatencyCheck(allowedOrigins ...string) http.HandlerFunc {
	origins := strings.Join(allowedOrigins, ",")
	return func(rw http.ResponseWriter, r *http.Request) {
		// Allowing timing information to be shared. This allows the browser
		// to exclude TLS handshake timing.
		rw.Header().Set("Timing-Allow-Origin", origins)
		rw.WriteHeader(http.StatusOK)
	}
}
