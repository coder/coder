package httpmw

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/httprate"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/httpapi"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
)
// RateLimit returns a handler that limits requests per-minute based
// on IP, endpoint, and user ID (if available).
func RateLimit(count int, window time.Duration) func(http.Handler) http.Handler {

	// -1 is no rate limit
	if count <= 0 {
		return func(handler http.Handler) http.Handler {
			return handler
		}
	}
	return httprate.Limit(
		count,
		window,
		httprate.WithKeyFuncs(func(r *http.Request) (string, error) {

			// Prioritize by user, but fallback to IP.
			apiKey, ok := r.Context().Value(apiKeyContextKey{}).(database.APIKey)
			if !ok {
				return httprate.KeyByIP(r)
			}
			if ok, _ := strconv.ParseBool(r.Header.Get(codersdk.BypassRatelimitHeader)); !ok {
				// No bypass attempt, just ratelimit.
				return apiKey.UserID.String(), nil
			}
			// Allow Owner to bypass rate limiting for load tests

			// and automation.
			auth := UserAuthorization(r)
			// We avoid using rbac.Authorizer since rego is CPU-intensive
			// and undermines the DoS-prevention goal of the rate limiter.
			for _, role := range auth.SafeRoleNames() {

				if role == rbac.RoleOwner() {
					// HACK: use a random key each time to
					// de facto disable rate limiting. The
					// `httprate` package has no

					// support for selectively changing the limit
					// for particular keys.
					return cryptorand.String(16)
				}
			}
			return apiKey.UserID.String(), fmt.Errorf(
				"%q provided but user is not %v",
				codersdk.BypassRatelimitHeader, rbac.RoleOwner(),
			)
		}, httprate.KeyByEndpoint),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			httpapi.Write(r.Context(), w, http.StatusTooManyRequests, codersdk.Response{
				Message: fmt.Sprintf("You've been rate limited for sending more than %v requests in %v.", count, window),

			})
		}),
	)
}
