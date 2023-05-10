package coderd

import (
	"net/http"
	"net/url"
	"strings"
)

func LatencyCheck(allowedOrigins ...*url.URL) http.HandlerFunc {
	allowed := make([]string, 0, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		// Allow the origin without a path
		tmp := *origin
		tmp.Path = ""
		allowed = append(allowed, strings.TrimSuffix(origin.String(), "/"))
	}
	origins := strings.Join(allowed, ",")
	return func(rw http.ResponseWriter, r *http.Request) {
		// Allowing timing information to be shared. This allows the browser
		// to exclude TLS handshake timing.
		rw.Header().Set("Timing-Allow-Origin", origins)
		rw.WriteHeader(http.StatusOK)
	}
}
