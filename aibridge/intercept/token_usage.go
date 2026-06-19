package intercept

import (
	"context"

	"cdr.dev/slog/v3"
)

// NonNegativeInputTokens returns the non-cached input token count (total minus cached), clamped to
// zero. Some providers violate the spec and report more cached tokens than total input tokens, which
// would yield a negative value and panic the Prometheus token counters downstream. When that happens
// it logs an error with the provider, endpoint, and constituent values so the offending response can
// be debugged.
func NonNegativeInputTokens(ctx context.Context, logger slog.Logger, provider, endpoint string, totalInputTokens, cachedInputTokens int64) int64 {
	input := totalInputTokens - cachedInputTokens
	if input < 0 {
		logger.Error(ctx, "provider reported more cached tokens than input tokens; clamping input token usage to zero",
			slog.F("provider", provider),
			slog.F("endpoint", endpoint),
			slog.F("total_input_tokens", totalInputTokens),
			slog.F("cached_input_tokens", cachedInputTokens),
		)
		return 0
	}
	return input
}
