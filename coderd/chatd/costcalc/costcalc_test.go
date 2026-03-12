package costcalc_test

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/coder/coder/v2/coderd/chatd/costcalc"
	"github.com/coder/coder/v2/codersdk"
)

func TestCalculateTotalCostMicros(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		usage codersdk.ChatMessageUsage
		cost  *codersdk.ModelCostConfig
		want  *decimal.Decimal
	}{
		{
			name:  "nil cost returns nil",
			usage: codersdk.ChatMessageUsage{InputTokens: int64Ptr(1000)},
			cost:  nil,
			want:  nil,
		},
		{
			name: "all priced usage fields nil returns nil",
			usage: codersdk.ChatMessageUsage{
				TotalTokens:  int64Ptr(1234),
				ContextLimit: int64Ptr(8192),
			},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens: decPtr("3"),
			},
			want: nil,
		},
		{
			name:  "single token preserves fractional micros",
			usage: codersdk.ChatMessageUsage{InputTokens: int64Ptr(1)},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens: decPtr("0.01"),
			},
			want: decPtr("0.01"),
		},
		{
			name:  "simple input only",
			usage: codersdk.ChatMessageUsage{InputTokens: int64Ptr(1000)},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens: decPtr("3"),
			},
			want: decPtr("3000"),
		},
		{
			name:  "simple output only",
			usage: codersdk.ChatMessageUsage{OutputTokens: int64Ptr(500)},
			cost: &codersdk.ModelCostConfig{
				OutputPricePerMillionTokens: decPtr("15"),
			},
			want: decPtr("7500"),
		},
		{
			name: "reasoning tokens billed at output rate",
			usage: codersdk.ChatMessageUsage{
				OutputTokens:    int64Ptr(300),
				ReasoningTokens: int64Ptr(200),
			},
			cost: &codersdk.ModelCostConfig{
				OutputPricePerMillionTokens: decPtr("15"),
			},
			want: decPtr("7500"),
		},
		{
			name:  "cache read tokens",
			usage: codersdk.ChatMessageUsage{CacheReadTokens: int64Ptr(10000)},
			cost: &codersdk.ModelCostConfig{
				CacheReadPricePerMillionTokens: decPtr("0.3"),
			},
			want: decPtr("3000"),
		},
		{
			name:  "cache creation tokens",
			usage: codersdk.ChatMessageUsage{CacheCreationTokens: int64Ptr(5000)},
			cost: &codersdk.ModelCostConfig{
				CacheWritePricePerMillionTokens: decPtr("3.75"),
			},
			want: decPtr("18750"),
		},
		{
			name: "full mixed usage totals all components exactly",
			usage: codersdk.ChatMessageUsage{
				InputTokens:         int64Ptr(101),
				OutputTokens:        int64Ptr(201),
				ReasoningTokens:     int64Ptr(52),
				CacheReadTokens:     int64Ptr(1005),
				CacheCreationTokens: int64Ptr(33),
				TotalTokens:         int64Ptr(1391),
				ContextLimit:        int64Ptr(4096),
			},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens:      decPtr("1.23"),
				OutputPricePerMillionTokens:     decPtr("4.56"),
				CacheReadPricePerMillionTokens:  decPtr("0.7"),
				CacheWritePricePerMillionTokens: decPtr("7.89"),
			},
			want: decPtr("2241.78"),
		},
		{
			name: "partial pricing only input contributes",
			usage: codersdk.ChatMessageUsage{
				InputTokens:         int64Ptr(1234),
				OutputTokens:        int64Ptr(999),
				ReasoningTokens:     int64Ptr(111),
				CacheReadTokens:     int64Ptr(500),
				CacheCreationTokens: int64Ptr(250),
			},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens: decPtr("2.5"),
			},
			want: decPtr("3085"),
		},
		{
			name:  "zero tokens with pricing returns zero pointer",
			usage: codersdk.ChatMessageUsage{InputTokens: int64Ptr(0)},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens: decPtr("3"),
			},
			want: decPtr("0"),
		},
		{
			name:  "non nil usage with nil prices returns zero pointer",
			usage: codersdk.ChatMessageUsage{InputTokens: int64Ptr(42)},
			cost:  &codersdk.ModelCostConfig{},
			want:  decPtr("0"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := costcalc.CalculateTotalCostMicros(tt.usage, tt.cost)

			assertEqualDecimalPtr(t, tt.want, got)
		})
	}
}

func assertEqualDecimalPtr(t *testing.T, want, got *decimal.Decimal) {
	t.Helper()

	switch {
	case want == nil || got == nil:
		requireSamePointerState(t, want, got)
	default:
		if !want.Equal(*got) {
			t.Fatalf("expected %s, got %s", want.String(), got.String())
		}
	}
}

func requireSamePointerState(t *testing.T, want, got *decimal.Decimal) {
	t.Helper()
	if want != got {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}

func decPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}
