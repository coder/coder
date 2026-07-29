package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/modelprices"
)

func TestFilterProviders(t *testing.T) {
	t.Parallel()

	rows := []modelprices.PriceRow{
		{Provider: "alibaba", Model: "qwen"},
		{Provider: "anthropic", Model: "claude"},
		{Provider: "openai", Model: "gpt"},
	}
	got := filterProviders(rows, supportedProviders)
	require.Len(t, got, 2)
	require.Equal(t, "anthropic", got[0].Provider)
	require.Equal(t, "openai", got[1].Provider)
}

func TestMissingProviders(t *testing.T) {
	t.Parallel()

	t.Run("Absent", func(t *testing.T) {
		t.Parallel()
		upstream := map[string]modelprices.UpstreamProvider{
			"openai": {Models: map[string]modelprices.UpstreamModel{
				"gpt-4o": {Cost: &modelprices.UpstreamCost{Input: floatPtr(2.5)}},
			}},
		}
		missing := missingProviders(upstream, []string{"anthropic", "openai"})
		require.Equal(t, []string{"anthropic"}, missing)
	})

	t.Run("EmptyModels", func(t *testing.T) {
		t.Parallel()
		upstream := map[string]modelprices.UpstreamProvider{
			"anthropic": {Models: map[string]modelprices.UpstreamModel{}},
			"openai": {Models: map[string]modelprices.UpstreamModel{
				"gpt-4o": {Cost: &modelprices.UpstreamCost{Input: floatPtr(2.5)}},
			}},
		}
		missing := missingProviders(upstream, []string{"anthropic", "openai"})
		require.Equal(t, []string{"anthropic"}, missing)
	})

	t.Run("None", func(t *testing.T) {
		t.Parallel()
		upstream := map[string]modelprices.UpstreamProvider{
			"anthropic": {Models: map[string]modelprices.UpstreamModel{
				"claude": {Cost: &modelprices.UpstreamCost{Input: floatPtr(3)}},
			}},
			"openai": {Models: map[string]modelprices.UpstreamModel{
				"gpt-4o": {Cost: &modelprices.UpstreamCost{Input: floatPtr(2.5)}},
			}},
		}
		missing := missingProviders(upstream, []string{"anthropic", "openai"})
		require.Empty(t, missing)
	})
}

// TestRunPrices covers the end-to-end prices pipeline: Transform all
// providers, filter to supportedProviders, reject missing providers, and
// produce sorted rows. Mirrors the original TestConvert assertions.
func TestRunPrices(t *testing.T) {
	t.Parallel()

	const upstreamJSON = `{
		"anthropic": {
			"models": {
				"claude-sonnet-4-7": {
					"cost": {"input": 3, "output": 15, "cache_read": 0.3, "cache_write": 3.75}
				},
				"claude-haiku": {
					"cost": {"input": 0.8, "output": 4}
				}
			}
		},
		"openai": {
			"models": {
				"gpt-4o": {"cost": {"input": 2.5, "output": 10, "cache_read": 1.25}},
				"gpt-no-prices": {}
			}
		},
		"alibaba": {
			"models": {
				"should-be-ignored": {"cost": {"input": 1, "output": 1}}
			}
		}
	}`

	var upstream map[string]modelprices.UpstreamProvider
	require.NoError(t, json.Unmarshal([]byte(upstreamJSON), &upstream))

	rows, err := runPricesRows([]byte(upstreamJSON), upstream)
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

	// Missing keys stay nil for OpenAI gpt-4o.
	gpt := rows[2]
	require.Equal(t, int64(2_500_000), *gpt.InputPrice)
	require.Equal(t, int64(10_000_000), *gpt.OutputPrice)
	require.Equal(t, int64(1_250_000), *gpt.CacheReadPrice)
	require.Nil(t, gpt.CacheWritePrice)
}

// TestRunPricesMissingProvider covers both shapes of "configured provider has
// no usable data": the provider's key is absent from upstream, or the key
// exists but its Models map is empty. Both should fail loud so we never
// ship a partial seed.
func TestRunPricesMissingProvider(t *testing.T) {
	t.Parallel()

	t.Run("Absent", func(t *testing.T) {
		t.Parallel()
		upstreamJSON := `{
			"openai": {
				"models": {
					"gpt-4o": {"cost": {"input": 2.5}}
				}
			}
		}`
		var upstream map[string]modelprices.UpstreamProvider
		require.NoError(t, json.Unmarshal([]byte(upstreamJSON), &upstream))
		_, err := runPricesRows([]byte(upstreamJSON), upstream)
		require.Error(t, err)
		require.Contains(t, err.Error(), "anthropic")
	})

	t.Run("EmptyModels", func(t *testing.T) {
		t.Parallel()
		upstreamJSON := `{
			"anthropic": {"models": {}},
			"openai": {
				"models": {
					"gpt-4o": {"cost": {"input": 2.5}}
				}
			}
		}`
		var upstream map[string]modelprices.UpstreamProvider
		require.NoError(t, json.Unmarshal([]byte(upstreamJSON), &upstream))
		_, err := runPricesRows([]byte(upstreamJSON), upstream)
		require.Error(t, err)
		require.Contains(t, err.Error(), "anthropic")
	})
}

func TestValidate(t *testing.T) {
	t.Parallel()

	t.Run("PassesWhenAnyRowHasPricing", func(t *testing.T) {
		t.Parallel()
		rows := []modelprices.PriceRow{
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
		rows := []modelprices.PriceRow{
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
