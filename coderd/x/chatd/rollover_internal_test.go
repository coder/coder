package chatd //nolint:testpackage // Exercises unexported rollover helpers.

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewContextWindowBudget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		window int64
		want   int64
	}{
		{1_000_000, 968_000},
		{200_000, 168_000},
		{128_000, 96_000},
		// From here the reserve is a quarter of the window.
		{32_000, 24_000},
		{8_000, 6_000},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("W%d", tc.window), func(t *testing.T) {
			t.Parallel()
			got := newContextWindowBudget(tc.window, 32_000, 70)
			require.Equal(t, tc.want, got.usableLimit)
		})
	}

	t.Run("SmallerOutputCapShrinksReserve", func(t *testing.T) {
		t.Parallel()
		got := newContextWindowBudget(200_000, 8_192, 70)
		require.Equal(t, int64(191_808), got.usableLimit)
	})
	t.Run("UnknownWindowHasNoPoints", func(t *testing.T) {
		t.Parallel()
		got := newContextWindowBudget(0, 32_000, 70)
		require.Equal(t, contextWindowBudget{thresholdPercent: 70}, got)
	})
}

func TestDecideRollover(t *testing.T) {
	t.Parallel()

	// W = 200k: C = 168k.
	budget := newContextWindowBudget(200_000, 32_000, 70)
	limit := rolloverDecision{action: rolloverActionRollover, reason: rolloverReasonLimit}
	tests := []struct {
		name   string
		tokens int64
		budget contextWindowBudget
		want   rolloverDecision
	}{
		{name: "DisabledAtHundred", tokens: 199_000, budget: newContextWindowBudget(200_000, 32_000, 100)},
		{name: "LimitAtUsableLimit", tokens: 168_000, budget: budget, want: limit},
		{name: "BelowLimit", tokens: 167_999, budget: budget},
		{name: "NoUsage", budget: budget},
		{name: "UnknownWindow", tokens: 1_000_000, budget: newContextWindowBudget(0, 32_000, 70)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, decideRollover(tc.tokens, tc.budget))
		})
	}
}
