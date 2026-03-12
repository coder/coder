package costcalc_test

import (
	"testing"

	"github.com/coder/coder/v2/coderd/chatd/costcalc"
	"github.com/coder/coder/v2/codersdk"
)

func TestCalculateTotalCostMicros(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		usage codersdk.ChatMessageUsage
		cost  *codersdk.ModelCostConfig
		want  *int64
	}{
		{
			name:  "nil cost returns nil",
			usage: codersdk.ChatMessageUsage{InputTokens: ptr(int64(1000))},
			cost:  nil,
			want:  nil,
		},
		{
			name: "all priced usage fields nil returns nil",
			usage: codersdk.ChatMessageUsage{
				TotalTokens:  ptr(int64(1234)),
				ContextLimit: ptr(int64(8192)),
			},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens: ptr(3.0),
			},
			want: nil,
		},
		{
			name:  "simple input only",
			usage: codersdk.ChatMessageUsage{InputTokens: ptr(int64(1000))},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens: ptr(3.0),
			},
			want: ptr(int64(3000)),
		},
		{
			name:  "simple output only",
			usage: codersdk.ChatMessageUsage{OutputTokens: ptr(int64(500))},
			cost: &codersdk.ModelCostConfig{
				OutputPricePerMillionTokens: ptr(15.0),
			},
			want: ptr(int64(7500)),
		},
		{
			name: "reasoning tokens billed at output rate",
			usage: codersdk.ChatMessageUsage{
				OutputTokens:    ptr(int64(300)),
				ReasoningTokens: ptr(int64(200)),
			},
			cost: &codersdk.ModelCostConfig{
				OutputPricePerMillionTokens: ptr(15.0),
			},
			want: ptr(int64(7500)),
		},
		{
			name:  "cache read tokens",
			usage: codersdk.ChatMessageUsage{CacheReadTokens: ptr(int64(10000))},
			cost: &codersdk.ModelCostConfig{
				CacheReadPricePerMillionTokens: ptr(0.3),
			},
			want: ptr(int64(3000)),
		},
		{
			name:  "cache creation tokens",
			usage: codersdk.ChatMessageUsage{CacheCreationTokens: ptr(int64(5000))},
			cost: &codersdk.ModelCostConfig{
				CacheWritePricePerMillionTokens: ptr(3.75),
			},
			want: ptr(int64(18750)),
		},
		{
			name: "full mixed usage sums rounded categories",
			usage: codersdk.ChatMessageUsage{
				InputTokens:         ptr(int64(101)),
				OutputTokens:        ptr(int64(201)),
				ReasoningTokens:     ptr(int64(52)),
				CacheReadTokens:     ptr(int64(1005)),
				CacheCreationTokens: ptr(int64(33)),
				TotalTokens:         ptr(int64(1391)),
				ContextLimit:        ptr(int64(4096)),
			},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens:      ptr(1.23),
				OutputPricePerMillionTokens:     ptr(4.56),
				CacheReadPricePerMillionTokens:  ptr(0.7),
				CacheWritePricePerMillionTokens: ptr(7.89),
			},
			want: ptr(int64(2242)),
		},
		{
			name: "partial pricing only input contributes",
			usage: codersdk.ChatMessageUsage{
				InputTokens:         ptr(int64(1234)),
				OutputTokens:        ptr(int64(999)),
				ReasoningTokens:     ptr(int64(111)),
				CacheReadTokens:     ptr(int64(500)),
				CacheCreationTokens: ptr(int64(250)),
			},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens: ptr(2.5),
			},
			want: ptr(int64(3085)),
		},
		{
			name:  "zero tokens with pricing returns zero pointer",
			usage: codersdk.ChatMessageUsage{InputTokens: ptr(int64(0))},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens: ptr(3.0),
			},
			want: ptr(int64(0)),
		},
		{
			name: "rounding happens per category",
			usage: codersdk.ChatMessageUsage{
				InputTokens:  ptr(int64(1)),
				OutputTokens: ptr(int64(1)),
			},
			cost: &codersdk.ModelCostConfig{
				InputPricePerMillionTokens:  ptr(0.15),
				OutputPricePerMillionTokens: ptr(0.15),
			},
			want: ptr(int64(0)),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := costcalc.CalculateTotalCostMicros(tt.usage, tt.cost)

			assertEqualInt64Ptr(t, tt.want, got)
		})
	}
}

func assertEqualInt64Ptr(t *testing.T, want, got *int64) {
	t.Helper()

	switch {
	case want == nil || got == nil:
		if want != got {
			t.Fatalf("expected %v, got %v", want, got)
		}
	default:
		if *want != *got {
			t.Fatalf("expected %d, got %d", *want, *got)
		}
	}
}

func ptr[T any](v T) *T {
	return &v
}
