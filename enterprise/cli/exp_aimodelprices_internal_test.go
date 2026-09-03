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
		{name: "Zero", price: new(int64(0)), want: "$0.00"},
		{name: "WholeDollars", price: new(int64(3_000_000)), want: "$3.00"},
		{name: "Fractional", price: new(int64(2_500_000)), want: "$2.50"},
		{name: "OneCent", price: new(int64(10_000)), want: "$0.01"},
		{name: "UnderACent", price: new(int64(3_600)), want: "$0.0036"},
		{name: "UnderACentTrailingZeros", price: new(int64(1_000)), want: "$0.001"},
		{name: "UnderACentManyDecimals", price: new(int64(3_625)), want: "$0.003625"},
		{name: "SmallestUnit", price: new(int64(1)), want: "$0.000001"},
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

	tests := []struct {
		name        string
		current     []codersdk.AIModelPrice
		requested   []codersdk.AIModelPriceUpsert
		wantAdded   int
		wantChanged int
	}{
		{
			name:    "UnknownModelIsAnAddition",
			current: nil,
			requested: []codersdk.AIModelPriceUpsert{{
				Provider: "anthropic", Model: "my-model", InputPrice: new(int64(100)),
			}},
			wantAdded:   1,
			wantChanged: 0,
		},
		{
			name: "ChangedPriceIsAChange",
			current: []codersdk.AIModelPrice{{
				Provider: "anthropic", Model: "my-model", InputPrice: new(int64(100)),
			}},
			requested: []codersdk.AIModelPriceUpsert{{
				Provider: "anthropic", Model: "my-model", InputPrice: new(int64(200)),
			}},
			wantAdded:   0,
			wantChanged: 1,
		},
		{
			name: "IdenticalPriceIsDropped",
			current: []codersdk.AIModelPrice{{
				Provider: "anthropic", Model: "my-model", InputPrice: new(int64(100)),
			}},
			requested: []codersdk.AIModelPriceUpsert{{
				Provider: "anthropic", Model: "my-model", InputPrice: new(int64(100)),
			}},
			wantAdded:   0,
			wantChanged: 0,
		},
		{
			name: "UnknownToValueIsAChange",
			current: []codersdk.AIModelPrice{{
				Provider: "anthropic", Model: "my-model", InputPrice: new(int64(100)),
			}},
			requested: []codersdk.AIModelPriceUpsert{{
				Provider: "anthropic", Model: "my-model", InputPrice: new(int64(100)), OutputPrice: new(int64(200)),
			}},
			wantAdded:   0,
			wantChanged: 1,
		},
		{
			name: "ValueToUnknownIsAChange",
			current: []codersdk.AIModelPrice{{
				Provider: "anthropic", Model: "my-model", InputPrice: new(int64(100)), OutputPrice: new(int64(200)),
			}},
			requested: []codersdk.AIModelPriceUpsert{{
				Provider: "anthropic", Model: "my-model", InputPrice: new(int64(100)),
			}},
			wantAdded:   0,
			wantChanged: 1,
		},
		{
			// A model of the same name under another provider is a different
			// row, so it does not match.
			name: "SameModelDifferentProvider",
			current: []codersdk.AIModelPrice{{
				Provider: "openai", Model: "my-model", InputPrice: new(int64(100)),
			}},
			requested: []codersdk.AIModelPriceUpsert{{
				Provider: "anthropic", Model: "my-model", InputPrice: new(int64(100)),
			}},
			wantAdded:   1,
			wantChanged: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			additions, changes := diffAIModelPrices(tt.requested, tt.current)
			require.Len(t, additions, tt.wantAdded)
			require.Len(t, changes, tt.wantChanged)

			for i, change := range changes {
				require.Equal(t, tt.requested[i], change.price)
				require.Equal(t, tt.current[i].InputPrice, change.old.InputPrice)
				require.Equal(t, tt.current[i].OutputPrice, change.old.OutputPrice)
			}
		})
	}
}
