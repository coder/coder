package aibridged

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/httprate"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

// OverloadConfig configures overload protection for the AI Bridge server.
type OverloadConfig struct {
	// MaxConcurrency is the maximum number of concurrent requests allowed.
	// Set to 0 to disable concurrency limiting.
	MaxConcurrency int64

	// RateLimit is the maximum number of requests per RateWindow.
	// Set to 0 to disable rate limiting.
	RateLimit int64

	// RateWindow is the duration of the rate limiting window.
	RateWindow time.Duration
}

// OverloadProtection provides middleware for protecting the AI Bridge server
// from overload conditions.
type OverloadProtection struct {
	config OverloadConfig
	logger slog.Logger

	// concurrencyLimiter tracks the number of concurrent requests.
	currentConcurrency atomic.Int64

	// rateLimiter is the rate limiting middleware.
	rateLimiter func(http.Handler) http.Handler
}

// NewOverloadProtection creates a new OverloadProtection instance.
func NewOverloadProtection(config OverloadConfig, logger slog.Logger) *OverloadProtection {
	op := &OverloadProtection{
		config: config,
		logger: logger.Named("overload"),
	}

	// Initialize rate limiter if configured.
	if config.RateLimit > 0 && config.RateWindow > 0 {
		op.rateLimiter = httprate.Limit(
			int(config.RateLimit),
			config.RateWindow,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
				httpapi.Write(r.Context(), w, http.StatusTooManyRequests, codersdk.Response{
					Message: "AI Bridge rate limit exceeded. Please try again later.",
				})
			}),
		)
	}

	return op
}

// ConcurrencyMiddleware returns a middleware that limits concurrent requests.
// Returns nil if concurrency limiting is disabled.
func (op *OverloadProtection) ConcurrencyMiddleware() func(http.Handler) http.Handler {
	if op.config.MaxConcurrency <= 0 {
		return nil
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			current := op.currentConcurrency.Add(1)
			defer op.currentConcurrency.Add(-1)

			if current > op.config.MaxConcurrency {
				op.logger.Warn(r.Context(), "ai bridge concurrency limit exceeded",
					slog.F("current", current),
					slog.F("max", op.config.MaxConcurrency),
				)
				httpapi.Write(r.Context(), w, http.StatusServiceUnavailable, codersdk.Response{
					Message: "AI Bridge is currently at capacity. Please try again later.",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitMiddleware returns a middleware that limits the rate of requests.
// Returns nil if rate limiting is disabled.
func (op *OverloadProtection) RateLimitMiddleware() func(http.Handler) http.Handler {
	return op.rateLimiter
}

// CurrentConcurrency returns the current number of concurrent requests.
func (op *OverloadProtection) CurrentConcurrency() int64 {
	return op.currentConcurrency.Load()
}

// WrapHandler wraps the given handler with all enabled overload protection
// middleware.
func (op *OverloadProtection) WrapHandler(handler http.Handler) http.Handler {
	// Apply rate limiting first (cheaper check).
	if op.rateLimiter != nil {
		handler = op.rateLimiter(handler)
	}

	// Then apply concurrency limiting.
	if concurrencyMW := op.ConcurrencyMiddleware(); concurrencyMW != nil {
		handler = concurrencyMW(handler)
	}

	return handler
}
