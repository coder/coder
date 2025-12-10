package httpmw

import (
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/go-chi/httprate"
	"golang.org/x/xerrors"

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
			auth := UserAuthorization(r.Context())

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

			return apiKey.UserID.String(), xerrors.Errorf(
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

// RateLimitByAuthToken returns a handler that limits requests based on the
// authentication token in the request.
//
// This differs from [RateLimit] in several ways:
//   - It extracts the token directly from request headers (Authorization Bearer
//     or X-Api-Key) rather than from the request context, making it suitable for
//     endpoints that handle authentication internally (like AI Bridge) rather than
//     via [ExtractAPIKeyMW] middleware.
//   - It does not support the bypass header for Owners.
//   - It does not key by endpoint, so the limit applies across all endpoints using
//     this middleware.
//   - It includes a Retry-After header in 429 responses for backpressure signaling.
//
// If no token is found in the headers, it falls back to rate limiting by IP address.
func RateLimitByAuthToken(count int, window time.Duration) func(http.Handler) http.Handler {
	if count <= 0 {
		return func(handler http.Handler) http.Handler {
			return handler
		}
	}

	return httprate.Limit(
		count,
		window,
		httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
			// Try to extract auth token for per-user rate limiting using
			// AI provider authentication headers.
			if token := ExtractAPIKeyFromHeader(r.Header); token != "" {
				return token, nil
			}
			// Fall back to IP-based rate limiting if no token present.
			return httprate.KeyByIP(r)
		}),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			// Add Retry-After header for backpressure signaling.
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(window.Seconds())))
			httpapi.Write(r.Context(), w, http.StatusTooManyRequests, codersdk.Response{
				Message: "You've been rate limited. Please try again later.",
			})
		}),
	)
}

// ConcurrencyLimit returns a handler that limits the number of concurrent
// requests. When the limit is exceeded, it returns HTTP 503 Service Unavailable.
func ConcurrencyLimit(maxConcurrent int64, resourceName string) func(http.Handler) http.Handler {
	if maxConcurrent <= 0 {
		return func(handler http.Handler) http.Handler {
			return handler
		}
	}

	var current atomic.Int64
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := current.Add(1)
			defer current.Add(-1)

			if c > maxConcurrent {
				httpapi.Write(r.Context(), w, http.StatusServiceUnavailable, codersdk.Response{
					Message: fmt.Sprintf("%s is currently at capacity. Please try again later.", resourceName),
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
