package keypool

import (
	"context"
	"net/http"

	"cdr.dev/slog/v3"
)

// MarkKeyOnStatus marks key based on a key-specific HTTP status
// code from resp (429 or 401 for temporary). Returns true if the
// status was a key-specific failover trigger so callers can retry
// with the next key.
func (p *Pool) MarkKeyOnStatus(
	ctx context.Context,
	key *Key,
	resp *http.Response,
	logger slog.Logger,
) bool {
	if resp == nil {
		return false
	}
	statusCode := resp.StatusCode
	switch statusCode {
	// A 429 rate-limits the key for the provider-supplied cooldown. A 401
	// means the key was rejected, so it cools down for the default period
	// and recovers on its own.
	case http.StatusTooManyRequests, http.StatusUnauthorized:
		cooldown := defaultCooldown
		reason := cooldownUnauthorized
		if statusCode == http.StatusTooManyRequests {
			reason = cooldownRateLimited
			if retryAfter := ParseRetryAfter(resp); retryAfter > 0 {
				cooldown = retryAfter
			}
		}
		if key.applyCooldown(cooldown, reason) {
			if p.metrics != nil {
				p.metrics.KeyPoolStateTransitions.WithLabelValues(p.providerName, string(reason)).Inc()
			}
			logger.Info(ctx, "key marked temporary",
				slog.F("provider", p.providerName),
				slog.F("api_key_hint", key.Hint()),
				slog.F("status", statusCode),
				slog.F("cooldown", cooldown))
		}
		return true
	default:
		logger.Debug(ctx, "status is not a key failover trigger",
			slog.F("provider", p.providerName),
			slog.F("status", statusCode))
		return false
	}
}
