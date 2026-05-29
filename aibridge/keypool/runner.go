package keypool

import (
	"context"

	"cdr.dev/slog/v3"
)

// Failover walks pool, invoking attempt with each candidate key until the
// attempt reports no failure (a nil *Failure) or the pool is exhausted. It
// owns key marking, the retry decision, and exhaustion.
//
// When an attempt returns a non-nil *Failure the chosen key is marked
// (temporary or permanent) and the next key is tried. The discarded payload
// is the closure's responsibility to clean up before reporting a failure. On
// exhaustion Failover returns the zero value of T and the pool's *Error.
func Failover[T any](
	ctx context.Context,
	pool *Pool,
	logger slog.Logger,
	providerName string,
	attempt func(ctx context.Context, key *Key) (T, *Failure),
) (T, *Error) {
	var zero T
	walker := pool.Walker()
	for {
		key, kpErr := walker.Next()
		if kpErr != nil {
			return zero, kpErr
		}

		payload, failure := attempt(ctx, key)
		if failure == nil {
			return payload, nil
		}

		switch failure.Reason {
		case FailoverRateLimited:
			if key.MarkTemporary(failure.Cooldown) {
				logger.Info(ctx, "key marked temporary",
					slog.F("provider", providerName),
					slog.F("api_key_hint", key.Hint()),
					slog.F("cooldown", failure.Cooldown))
			}
		case FailoverUnauthorized, FailoverForbidden:
			if key.MarkPermanent() {
				logger.Warn(ctx, "key marked permanent",
					slog.F("provider", providerName),
					slog.F("api_key_hint", key.Hint()))
			}
		}
	}
}
