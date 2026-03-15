package chatcost_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chatcost"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

func TestCalculateTotalCostMicros(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		usage      codersdk.ChatMessageUsage
		cost       *codersdk.ModelCostConfig
		wantMicros int64
		wantValid  bool
	}{
		{
			name:       "nil cost returns unpriced",
			usage:      codersdk.ChatMessageUsage{InputTokens: ptr.Ref[int64](1000)},
			cost:       nil,
			wantMicros: 0,
			wantValid:  false,
		},
		{
			name: "all priced usage fields nil returns unpriced",
			usage: codersdk.ChatMessageUsage{
				TotalTokens:  ptr.Ref[int64](1234),
				ContextLimit: ptr.Ref[int64](8192),
			},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens: ptr.Ref(decimal.RequireFromString("3")),
			},
			wantMicros: 0,
			wantValid:  false,
		},
		{
			name:       "sub-micro total rounds up to 1",
			usage:      codersdk.ChatMessageUsage{InputTokens: ptr.Ref[int64](1)},
			cost:       &codersdk.ModelCostConfig{InputPricePerMillionTokens: ptr.Ref(decimal.RequireFromString("0.01"))},
			wantMicros: 1,
			wantValid:  true,
		},
		{
			name:       "simple input only",
			usage:      codersdk.ChatMessageUsage{InputTokens: ptr.Ref[int64](1000)},
			cost:       &codersdk.ModelCostConfig{InputPricePerMillionTokens: ptr.Ref(decimal.RequireFromString("3"))},
			wantMicros: 3000,
			wantValid:  true,
		},
		{
			name:       "simple output only",
			usage:      codersdk.ChatMessageUsage{OutputTokens: ptr.Ref[int64](500)},
			cost:       &codersdk.ModelCostConfig{OutputPricePerMillionTokens: ptr.Ref(decimal.RequireFromString("15"))},
			wantMicros: 7500,
			wantValid:  true,
		},
		{
			name: "reasoning tokens included in output total",
			usage: codersdk.ChatMessageUsage{
				OutputTokens:    ptr.Ref[int64](500),
				ReasoningTokens: ptr.Ref[int64](200),
			},
			cost:       &codersdk.ModelCostConfig{OutputPricePerMillionTokens: ptr.Ref(decimal.RequireFromString("15"))},
			wantMicros: 7500,
			wantValid:  true,
		},
		{
			name:       "cache read tokens",
			usage:      codersdk.ChatMessageUsage{CacheReadTokens: ptr.Ref[int64](10000)},
			cost:       &codersdk.ModelCostConfig{CacheReadPricePerMillionTokens: ptr.Ref(decimal.RequireFromString("0.3"))},
			wantMicros: 3000,
			wantValid:  true,
		},
		{
			name:       "cache creation tokens",
			usage:      codersdk.ChatMessageUsage{CacheCreationTokens: ptr.Ref[int64](5000)},
			cost:       &codersdk.ModelCostConfig{CacheWritePricePerMillionTokens: ptr.Ref(decimal.RequireFromString("3.75"))},
			wantMicros: 18750,
			wantValid:  true,
		},
		{
			name: "full mixed usage totals all components exactly",
			usage: codersdk.ChatMessageUsage{
				InputTokens:         ptr.Ref[int64](101),
				OutputTokens:        ptr.Ref[int64](201),
				ReasoningTokens:     ptr.Ref[int64](52),
				CacheReadTokens:     ptr.Ref[int64](1005),
				CacheCreationTokens: ptr.Ref[int64](33),
				TotalTokens:         ptr.Ref[int64](1391),
				ContextLimit:        ptr.Ref[int64](4096),
			},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens:      ptr.Ref(decimal.RequireFromString("1.23")),
				OutputPricePerMillionTokens:     ptr.Ref(decimal.RequireFromString("4.56")),
				CacheReadPricePerMillionTokens:  ptr.Ref(decimal.RequireFromString("0.7")),
				CacheWritePricePerMillionTokens: ptr.Ref(decimal.RequireFromString("7.89")),
			},
			wantMicros: 2005,
			wantValid:  true,
		},
		{
			name: "partial pricing only input contributes",
			usage: codersdk.ChatMessageUsage{
				InputTokens:         ptr.Ref[int64](1234),
				OutputTokens:        ptr.Ref[int64](999),
				ReasoningTokens:     ptr.Ref[int64](111),
				CacheReadTokens:     ptr.Ref[int64](500),
				CacheCreationTokens: ptr.Ref[int64](250),
			},
			cost:       &codersdk.ModelCostConfig{InputPricePerMillionTokens: ptr.Ref(decimal.RequireFromString("2.5"))},
			wantMicros: 3085,
			wantValid:  true,
		},
		{
			name:       "zero tokens with pricing returns zero cost",
			usage:      codersdk.ChatMessageUsage{InputTokens: ptr.Ref[int64](0)},
			cost:       &codersdk.ModelCostConfig{InputPricePerMillionTokens: ptr.Ref(decimal.RequireFromString("3"))},
			wantMicros: 0,
			wantValid:  true,
		},
		{
			name:       "usage only in unpriced categories returns unpriced",
			usage:      codersdk.ChatMessageUsage{InputTokens: ptr.Ref[int64](1000)},
			cost:       &codersdk.ModelCostConfig{OutputPricePerMillionTokens: ptr.Ref(decimal.RequireFromString("15"))},
			wantMicros: 0,
			wantValid:  false,
		},
		{
			name:       "non nil usage with empty cost config returns unpriced",
			usage:      codersdk.ChatMessageUsage{InputTokens: ptr.Ref[int64](42)},
			cost:       &codersdk.ModelCostConfig{},
			wantMicros: 0,
			wantValid:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			micros, valid := chatcost.CalculateTotalCostMicros(tt.usage, tt.cost)

			require.Equal(t, tt.wantValid, valid)
			require.Equal(t, tt.wantMicros, micros)
		})
	}
}
