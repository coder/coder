package keypool

import (
	"context"
	"net/http"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/utils"
)

// MarkKeyOnStatus marks key based on a key-specific HTTP
// status code (429 for temporary, 401 or 403 for permanent).
// Returns true if the status was a key-specific failover
// trigger so callers can retry with the next key.
func MarkKeyOnStatus(
	ctx context.Context,
	key *Key,
	statusCode int,
	resp *http.Response,
	logger slog.Logger,
	providerName string,
) bool {
	switch statusCode {
	case http.StatusTooManyRequests:
		cooldown := ParseRetryAfter(resp)
		if key.MarkTemporary(cooldown) {
			logger.Warn(ctx, "key marked temporary",
				slog.F("provider", providerName),
				slog.F("api_key_hint", utils.MaskSecret(key.Value())),
				slog.F("status", statusCode),
				slog.F("cooldown", cooldown))
		}
		return true
	case http.StatusUnauthorized, http.StatusForbidden:
		if key.MarkPermanent() {
			logger.Error(ctx, "key marked permanent",
				slog.F("provider", providerName),
				slog.F("api_key_hint", utils.MaskSecret(key.Value())),
				slog.F("status", statusCode))
		}
		return true
	default:
		logger.Debug(ctx, "status is not a key failover trigger",
			slog.F("provider", providerName),
			slog.F("status", statusCode))
		return false
	}
}
