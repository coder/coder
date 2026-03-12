package costcalc

import (
	"github.com/shopspring/decimal"

	"github.com/coder/coder/v2/codersdk"
)

// CalculateTotalCostMicros computes the total cost of a chat message in
// whole micros (millionths of a dollar) using the configured model pricing.
//
// Fractional micro components are summed with decimal precision and then
// rounded once at the end so callers receive a persistable whole-micro value.
//
// Returns nil when pricing is not configured or when all priced usage fields
// are nil, allowing callers to distinguish "zero cost" from "unpriced".
func CalculateTotalCostMicros(
	usage codersdk.ChatMessageUsage,
	cost *codersdk.ModelCostConfig,
) *int64 {
	if cost == nil {
		return nil
	}

	if usage.InputTokens == nil &&
		usage.OutputTokens == nil &&
		usage.ReasoningTokens == nil &&
		usage.CacheCreationTokens == nil &&
		usage.CacheReadTokens == nil {
		return nil
	}

	inputMicros := decimal.NewFromInt(derefInt64(usage.InputTokens)).
		Mul(derefDecimal(cost.InputPricePerMillionTokens))
	outputMicros := decimal.NewFromInt(derefInt64(usage.OutputTokens) + derefInt64(usage.ReasoningTokens)).
		Mul(derefDecimal(cost.OutputPricePerMillionTokens))
	cacheReadMicros := decimal.NewFromInt(derefInt64(usage.CacheReadTokens)).
		Mul(derefDecimal(cost.CacheReadPricePerMillionTokens))
	cacheWriteMicros := decimal.NewFromInt(derefInt64(usage.CacheCreationTokens)).
		Mul(derefDecimal(cost.CacheWritePricePerMillionTokens))

	total := inputMicros.
		Add(outputMicros).
		Add(cacheReadMicros).
		Add(cacheWriteMicros)
	rounded := total.Round(0).IntPart()
	return &rounded
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}

	return *v
}

func derefDecimal(v *decimal.Decimal) decimal.Decimal {
	if v == nil {
		return decimal.Zero
	}

	return *v
}
