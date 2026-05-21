package coderd

import (
	"net/http"
)

// LatencyCheck is an endpoint for the web ui to measure latency with.
// allowAll allows any Origin to get timing information. The allowAll should
// only be set in dev modes.
//
//nolint:revive
func LatencyCheck() http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		// Allowing timing information to be shared. This allows the browser
		// to exclude TLS handshake timing.
		rw.Header().Set("Timing-Allow-Origin", "*")
		// Always allow all CORs on this route.
		rw.Header().Set("Access-Control-Allow-Origin", "*")
		rw.Header().Set("Access-Control-Allow-Headers", "*")
		rw.Header().Set("Access-Control-Allow-Credentials", "false")
		rw.Header().Set("Access-Control-Allow-Methods", "*")
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("OK"))
	}
}
