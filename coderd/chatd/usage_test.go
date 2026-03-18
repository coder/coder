package chatd //nolint:testpackage // Uses internal symbols.

import (
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"
)

func TestNormalizePersistedUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		provider            string
		raw                 fantasy.Usage
		wantInputTokens     int64
		wantCacheReadTokens int64
	}{
		{
			name:     "OpenAI subtracts cached tokens",
			provider: "openai",
			raw: fantasy.Usage{
				InputTokens:     1000,
				CacheReadTokens: 900,
			},
			wantInputTokens:     100,
			wantCacheReadTokens: 900,
		},
		{
			name:     "Azure clamps below zero",
			provider: "azure",
			raw: fantasy.Usage{
				InputTokens:     500,
				CacheReadTokens: 600,
			},
			wantInputTokens:     0,
			wantCacheReadTokens: 600,
		},
		{
			name:     "Anthropic usage is unchanged",
			provider: "anthropic",
			raw: fantasy.Usage{
				InputTokens:     1000,
				CacheReadTokens: 900,
			},
			wantInputTokens:     1000,
			wantCacheReadTokens: 900,
		},
		{
			name:     "OpenAI without cache is unchanged",
			provider: "openai",
			raw: fantasy.Usage{
				InputTokens: 500,
			},
			wantInputTokens:     500,
			wantCacheReadTokens: 0,
		},
		{
			name:     "OpenAI with explicit zero cache",
			provider: "openai",
			raw: fantasy.Usage{
				InputTokens:     500,
				CacheReadTokens: 0,
				OutputTokens:    100,
			},
			wantInputTokens:     500,
			wantCacheReadTokens: 0,
		},
		{
			name:     "OpenAI both zero stays zero",
			provider: "openai",
			raw: fantasy.Usage{
				InputTokens:     0,
				CacheReadTokens: 0,
			},
			wantInputTokens:     0,
			wantCacheReadTokens: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := tt.raw
			normalized := normalizePersistedUsage(tt.provider, tt.raw)

			require.Equal(t, tt.wantInputTokens, normalized.InputTokens)
			require.Equal(t, tt.wantCacheReadTokens, normalized.CacheReadTokens)
			require.Equal(t, original, tt.raw, "normalizePersistedUsage must not mutate the input value")
		})
	}
}
