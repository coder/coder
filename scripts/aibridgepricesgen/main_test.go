package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/aibridge/prices/pricebook"
)

func TestToMicros(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   *float64
		want *int64
	}{
		{"missing", nil, nil},
		{"zero", floatPtr(0), int64Ptr(0)},
		{"whole", floatPtr(3), int64Ptr(3_000_000)},
		{"fractional", floatPtr(0.075), int64Ptr(75_000)},
		{"negative", floatPtr(-1), nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := toMicros(tc.in)
			if tc.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tc.want, *got)
		})
	}
}

func TestConvert(t *testing.T) {
	t.Parallel()

	const upstreamJSON = `{
		"anthropic": {
			"models": {
				"claude-sonnet-4-7": {
					"cost": {
						"input": 3, "output": 15, "cache_read": 0.3, "cache_write": 3.75,
						"tiers": [
							{"input": 12, "output": 40, "tier": {"type": "context", "size": 500000}},
							{"input": 6, "output": 22.5, "cache_read": 0.6, "cache_write": 7.5, "tier": {"type": "context", "size": 200000}}
						],
						"context_over_200k": {"input": 99, "output": 99}
					}
				},
				"claude-haiku": {
					"cost": {"input": 0.8, "output": 4}
				}
			}
		},
		"openai": {
			"models": {
				"gpt-4o": {"cost": {
					"input": 2.5, "output": 10, "cache_read": 1.25,
					"tiers": [{"input": 5, "output": 20, "tier": {"type": "context", "size": 272000}}]
				}},
				"gpt-no-prices": {}
			}
		},
		"alibaba": {
			"models": {
				"should-be-ignored": {"cost": {"input": 1, "output": 1}}
			}
		}
	}`

	var upstream map[string]upstreamProvider
	require.NoError(t, json.Unmarshal([]byte(upstreamJSON), &upstream))

	rows, err := convert(upstream, []string{"anthropic", "openai"})
	require.NoError(t, err)

	// alibaba is dropped (not a supported provider) and gpt-no-prices is
	// dropped (no per-token pricing), leaving three priced rows.
	require.Len(t, rows, 3)

	// Sorted (provider, model).
	require.Equal(t, "anthropic", rows[0].Provider)
	require.Equal(t, "claude-haiku", rows[0].Model)
	require.Equal(t, "anthropic", rows[1].Provider)
	require.Equal(t, "claude-sonnet-4-7", rows[1].Model)
	require.Equal(t, "openai", rows[2].Provider)
	require.Equal(t, "gpt-4o", rows[2].Model)

	// All four prices populated for Anthropic Sonnet.
	sonnet := rows[1]
	require.Equal(t, int64(3_000_000), *sonnet.InputPrice)
	require.Equal(t, int64(15_000_000), *sonnet.OutputPrice)
	require.Equal(t, int64(300_000), *sonnet.CacheReadPrice)
	require.Equal(t, int64(3_750_000), *sonnet.CacheWritePrice)
	require.Equal(t, int64(6_000_000), *sonnet.CacheWrite1hPrice)
	// Tiers are sorted by size, keep absent prices nil, and get a 1h price
	// of 2x their own input. context_over_200k is not read.
	require.Equal(t, []pricebook.Tier{
		{
			ThresholdTokens:   200_000,
			InputPrice:        int64Ptr(6_000_000),
			OutputPrice:       int64Ptr(22_500_000),
			CacheReadPrice:    int64Ptr(600_000),
			CacheWritePrice:   int64Ptr(7_500_000),
			CacheWrite1hPrice: int64Ptr(12_000_000),
		},
		{
			ThresholdTokens:   500_000,
			InputPrice:        int64Ptr(12_000_000),
			OutputPrice:       int64Ptr(40_000_000),
			CacheWrite1hPrice: int64Ptr(24_000_000),
		},
	}, sonnet.Tiers)
	require.Empty(t, rows[0].Tiers, "claude-haiku has no tiers")

	// Missing keys stay nil for OpenAI gpt-4o.
	gpt := rows[2]
	require.Equal(t, int64(2_500_000), *gpt.InputPrice)
	require.Equal(t, int64(10_000_000), *gpt.OutputPrice)
	require.Equal(t, int64(1_250_000), *gpt.CacheReadPrice)
	require.Nil(t, gpt.CacheWritePrice)
	require.Nil(t, gpt.CacheWrite1hPrice)
	require.Equal(t, []pricebook.Tier{{
		ThresholdTokens: 272_000,
		InputPrice:      int64Ptr(5_000_000),
		OutputPrice:     int64Ptr(20_000_000),
	}}, gpt.Tiers)
}

func TestClaude1hPrice(t *testing.T) {
	t.Parallel()

	cases := []struct {
		provider, model string
		input           *int64
		want            *int64
	}{
		{"anthropic", "claude-opus-5-5", int64Ptr(4_000_000), int64Ptr(8_000_000)},
		{"bedrock", "anthropic.claude-sonnet-4-6", int64Ptr(3_000_000), int64Ptr(6_000_000)},
		{"copilot", "claude-sonnet-4.6", int64Ptr(3_000_000), int64Ptr(6_000_000)},
		// Other providers serve Claude through paths that do not report the
		// 1h part of cache writes.
		{"openrouter", "anthropic/claude-sonnet-4", int64Ptr(3_000_000), nil},
		{"vercel", "anthropic/claude-sonnet-4", int64Ptr(3_000_000), nil},
		{"azure", "claude-opus-5-5", int64Ptr(4_000_000), nil},
		{"bedrock", "amazon.nova-pro", int64Ptr(800_000), nil},
		{"anthropic", "claude-unpriced", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.provider+"/"+tc.model, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, claude1hPrice(tc.provider, tc.model, tc.input))
		})
	}
}

func TestConvertTiersRejectsInvalid(t *testing.T) {
	t.Parallel()

	tier := func(typ string, size int64) upstreamTier {
		var tier upstreamTier
		tier.Input = floatPtr(1)
		tier.Tier.Type = typ
		tier.Tier.Size = size
		return tier
	}
	cases := []struct {
		name  string
		tiers []upstreamTier
		want  string
	}{
		{"UnknownType", []upstreamTier{tier("output", 1000)}, `unknown tier type "output"`},
		{"ZeroSize", []upstreamTier{tier("context", 0)}, "tier size 0 is not positive"},
		{"NegativeSize", []upstreamTier{tier("context", -1)}, "tier size -1 is not positive"},
		{"DuplicateSize", []upstreamTier{tier("context", 1000), tier("context", 1000)}, "duplicate tier size 1000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := convertTiers("openai", "gpt", tc.tiers)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

// TestConvertMissingProvider covers both shapes of "configured provider has
// no usable data": the provider's key is absent from upstream, or the key
// exists but its Models map is empty. Both should fail loud so we never
// ship a partial seed.
func TestConvertMissingProvider(t *testing.T) {
	t.Parallel()

	t.Run("Absent", func(t *testing.T) {
		t.Parallel()
		upstream := map[string]upstreamProvider{
			"openai": {Models: map[string]upstreamModel{
				"gpt-4o": {Cost: &upstreamCost{Input: floatPtr(2.5)}},
			}},
		}
		rows, err := convert(upstream, []string{"anthropic", "openai"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "anthropic")
		require.Nil(t, rows)
	})

	t.Run("EmptyModels", func(t *testing.T) {
		t.Parallel()
		upstream := map[string]upstreamProvider{
			"anthropic": {Models: map[string]upstreamModel{}},
			"openai": {Models: map[string]upstreamModel{
				"gpt-4o": {Cost: &upstreamCost{Input: floatPtr(2.5)}},
			}},
		}
		rows, err := convert(upstream, []string{"anthropic", "openai"})
		require.Error(t, err)
		require.Contains(t, err.Error(), "anthropic")
		require.Nil(t, rows)
	})
}

func TestValidate(t *testing.T) {
	t.Parallel()

	t.Run("PassesWhenAnyRowHasPricing", func(t *testing.T) {
		t.Parallel()
		rows := []pricebook.Row{
			{Provider: "openai", Model: "no-prices"},
			{Provider: "anthropic", Model: "claude", InputPrice: int64Ptr(3_000_000)},
		}
		require.NoError(t, validate(rows))
	})

	t.Run("FailsWhenNoRowHasPricing", func(t *testing.T) {
		t.Parallel()
		// Mirrors what would happen if upstream renamed the `cost` key:
		// Go's decoder silently drops it, every row gets all-null prices,
		// and convert returns syntactically valid rows with no pricing.
		rows := []pricebook.Row{
			{Provider: "anthropic", Model: "claude-x"},
			{Provider: "openai", Model: "gpt-x"},
		}
		err := validate(rows)
		require.Error(t, err)
		require.Contains(t, err.Error(), "converted rows have no pricing data")
	})
}

func floatPtr(v float64) *float64 { return &v }
func int64Ptr(v int64) *int64     { return &v }
