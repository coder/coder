package keypool

import (
	"context"
	"net/http"

	"cdr.dev/slog/v3"
)

// MarkKeyOnStatus marks key based on a key-specific HTTP
// status code from resp (429 for temporary, 401 or 403 for
// permanent). Returns true if the status was a key-specific
// failover trigger so callers can retry with the next key.
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
	case http.StatusTooManyRequests:
		cooldown := ParseRetryAfter(resp)
		if cooldown <= 0 {
			cooldown = defaultCooldown
		}
		if key.MarkTemporary(cooldown) {
			if p.metrics != nil {
				p.metrics.KeyPoolStateTransitions.WithLabelValues(p.providerName, reasonRateLimited).Inc()
			}
			logger.Info(ctx, "key marked temporary",
				slog.F("provider", p.providerName),
				slog.F("api_key_hint", key.Hint()),
				slog.F("status", statusCode),
				slog.F("cooldown", cooldown))
		}
		return true
	case http.StatusUnauthorized, http.StatusForbidden:
		if key.MarkPermanent() {
			if p.metrics != nil {
				reason := reasonUnauthorized
				if statusCode == http.StatusForbidden {
					reason = reasonForbidden
				}
				p.metrics.KeyPoolStateTransitions.WithLabelValues(p.providerName, reason).Inc()
			}
			logger.Warn(ctx, "key marked permanent",
				slog.F("provider", p.providerName),
				slog.F("api_key_hint", key.Hint()),
				slog.F("status", statusCode))
		}
		return true
	default:
		logger.Debug(ctx, "status is not a key failover trigger",
			slog.F("provider", p.providerName),
			slog.F("status", statusCode))
		return false
	}
}
