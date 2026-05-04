package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
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

	var upstream map[string]upstreamProvider
	require.NoError(t, json.Unmarshal([]byte(upstreamJSON), &upstream))

	rows := convert(upstream, []string{"anthropic", "openai"})

	// Filtered: alibaba dropped, four rows from the two allowed providers.
	require.Len(t, rows, 4)

	// Sorted (provider, model).
	require.Equal(t, "anthropic", rows[0].Provider)
	require.Equal(t, "claude-haiku", rows[0].Model)
	require.Equal(t, "anthropic", rows[1].Provider)
	require.Equal(t, "claude-sonnet-4-7", rows[1].Model)
	require.Equal(t, "openai", rows[2].Provider)
	require.Equal(t, "gpt-4o", rows[2].Model)
	require.Equal(t, "openai", rows[3].Provider)
	require.Equal(t, "gpt-no-prices", rows[3].Model)

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

	// Model with no cost block at all retains its row, all prices nil.
	empty := rows[3]
	require.Nil(t, empty.InputPrice)
	require.Nil(t, empty.OutputPrice)
	require.Nil(t, empty.CacheReadPrice)
	require.Nil(t, empty.CacheWritePrice)
}

func TestConvertMissingProvider(t *testing.T) {
	t.Parallel()

	upstream := map[string]upstreamProvider{
		"openai": {Models: map[string]upstreamModel{
			"gpt-4o": {Cost: &upstreamCost{Input: floatPtr(2.5)}},
		}},
	}
	// "anthropic" not present in upstream — should be skipped without panic.
	rows := convert(upstream, []string{"anthropic", "openai"})
	require.Len(t, rows, 1)
	require.Equal(t, "openai", rows[0].Provider)
}

func floatPtr(v float64) *float64 { return &v }
func int64Ptr(v int64) *int64     { return &v }
