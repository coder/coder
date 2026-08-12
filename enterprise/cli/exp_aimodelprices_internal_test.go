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
		{name: "OneCent", price: ptr(int64(10_000)), want: "$0.01"},
		{name: "UnderACent", price: ptr(int64(3_600)), want: "$0.0036"},
		{name: "UnderACentTrailingZeros", price: ptr(int64(1_000)), want: "$0.001"},
		{name: "UnderACentManyDecimals", price: ptr(int64(3_625)), want: "$0.003625"},
		{name: "SmallestUnit", price: ptr(int64(1)), want: "$0.000001"},
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

			// A change carries the requested price alongside the row it
			// replaces, which is what the preview renders as "old -> new".
			for i, change := range changes {
				require.Equal(t, tt.requested[i], change.price)
				require.Equal(t, tt.current[i].InputPrice, change.old.InputPrice)
				require.Equal(t, tt.current[i].OutputPrice, change.old.OutputPrice)
			}
		})
	}
}

func ptr(v int64) *int64 {
	return &v
}
