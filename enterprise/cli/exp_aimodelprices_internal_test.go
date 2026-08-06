package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestFormatMicros(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		price *int64
		want  string
	}{
		{name: "Unknown", price: nil, want: "-"},
		{name: "Zero", price: ptr(int64(0)), want: "$0.00"},
		{name: "WholeDollars", price: ptr(int64(3_000_000)), want: "$3.00"},
		{name: "Fractional", price: ptr(int64(2_500_000)), want: "$2.50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, formatMicros(tt.price))
		})
	}
}

func TestDiffAIModelPrices(t *testing.T) {
	t.Parallel()

	stored := func(input, output *int64) codersdk.AIModelPrice {
		return codersdk.AIModelPrice{
			Provider:    "anthropic",
			Model:       "my-model",
			InputPrice:  input,
			OutputPrice: output,
		}
	}
	requested := func(input, output *int64) codersdk.AIModelPriceUpsert {
		return codersdk.AIModelPriceUpsert{
			Provider:    "anthropic",
			Model:       "my-model",
			InputPrice:  input,
			OutputPrice: output,
		}
	}

	tests := []struct {
		name        string
		requested   []codersdk.AIModelPriceUpsert
		current     []codersdk.AIModelPrice
		wantAdded   int
		wantChanged int
	}{
		{
			name:      "UnknownModelIsAnAddition",
			requested: []codersdk.AIModelPriceUpsert{requested(ptr(int64(100)), nil)},
			current:   nil,
			wantAdded: 1,
		},
		{
			name:        "ChangedPriceIsAChange",
			requested:   []codersdk.AIModelPriceUpsert{requested(ptr(int64(200)), nil)},
			current:     []codersdk.AIModelPrice{stored(ptr(int64(100)), nil)},
			wantChanged: 1,
		},
		{
			name:      "IdenticalPriceIsDropped",
			requested: []codersdk.AIModelPriceUpsert{requested(ptr(int64(100)), nil)},
			current:   []codersdk.AIModelPrice{stored(ptr(int64(100)), nil)},
		},
		{
			name:        "UnknownToValueIsAChange",
			requested:   []codersdk.AIModelPriceUpsert{requested(ptr(int64(100)), ptr(int64(200)))},
			current:     []codersdk.AIModelPrice{stored(ptr(int64(100)), nil)},
			wantChanged: 1,
		},
		{
			name:        "ValueToUnknownIsAChange",
			requested:   []codersdk.AIModelPriceUpsert{requested(ptr(int64(100)), nil)},
			current:     []codersdk.AIModelPrice{stored(ptr(int64(100)), ptr(int64(200)))},
			wantChanged: 1,
		},
		{
			// A model of the same name under another provider is a different
			// row, so it does not match.
			name:      "SameModelDifferentProvider",
			requested: []codersdk.AIModelPriceUpsert{requested(ptr(int64(100)), nil)},
			current: []codersdk.AIModelPrice{{
				Provider: "openai", Model: "my-model", InputPrice: ptr(int64(100)),
			}},
			wantAdded: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			additions, changes := diffAIModelPrices(tt.requested, tt.current)
			require.Len(t, additions, tt.wantAdded)
			require.Len(t, changes, tt.wantChanged)
		})
	}
}

func ptr(v int64) *int64 {
	return &v
}
