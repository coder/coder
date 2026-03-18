package chatd

import "charm.land/fantasy"

// normalizePersistedUsage returns a copy of raw with provider-specific
// adjustments for persistence. For OpenAI and Azure, InputTokens includes
// cached tokens; subtract CacheReadTokens so persisted InputTokens represents
// only non-cached input. This avoids double-counting when the cost
// calculator prices both input and cache-read buckets additively.
func normalizePersistedUsage(provider string, raw fantasy.Usage) fantasy.Usage {
	normalized := raw

	switch provider {
	case "openai", "azure":
		if raw.InputTokens <= raw.CacheReadTokens {
			normalized.InputTokens = 0
			return normalized
		}
		normalized.InputTokens = raw.InputTokens - raw.CacheReadTokens
	}

	return normalized
}
