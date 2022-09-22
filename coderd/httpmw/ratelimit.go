package httpmw

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

// RateLimitPerMinute returns a handler that limits requests per-minute based
// on IP, endpoint, and user ID (if available).
func RateLimitPerMinute(count int) func(http.Handler) http.Handler {
	// -1 is no rate limit
	if count <= 0 {
		return func(handler http.Handler) http.Handler {
			return handler
		}
	}
	return httprate.Limit(
		count,
		1*time.Minute,
		httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
			// Prioritize by user, but fallback to IP.
			apiKey, ok := r.Context().Value(apiKeyContextKey{}).(database.APIKey)
			if ok {
				return apiKey.UserID.String(), nil
			}
			return httprate.KeyByIP(r)
		}, httprate.KeyByEndpoint),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(r.Context(), w, http.StatusTooManyRequests, codersdk.Response{
				Message: "You've been rate limited for sending too many requests!",
			})
		}),
	)
}
