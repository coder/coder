package chatcost

import (
	"github.com/shopspring/decimal"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// Returns cost in micros -- millionths of a dollar, rounded up to the next
// whole microdollar.
// Returns nil when pricing is not configured or when all priced usage fields
// are nil, allowing callers to distinguish "zero cost" from "unpriced".
func CalculateTotalCostMicros(
	usage codersdk.ChatMessageUsage,
	cost *codersdk.ModelCostConfig,
) *int64 {
	if cost == nil {
		return nil
	}

	// A cost config with no prices set means pricing is effectively
	// unconfigured — return nil (unpriced) rather than zero.
	if cost.InputPricePerMillionTokens == nil &&
		cost.OutputPricePerMillionTokens == nil &&
		cost.CacheReadPricePerMillionTokens == nil &&
		cost.CacheWritePricePerMillionTokens == nil {
		return nil
	}

	if usage.InputTokens == nil &&
		usage.OutputTokens == nil &&
		usage.ReasoningTokens == nil &&
		usage.CacheCreationTokens == nil &&
		usage.CacheReadTokens == nil {
		return nil
	}

	// OutputTokens already includes reasoning tokens per provider
	// semantics (e.g. OpenAI's completion_tokens encompasses
	// reasoning_tokens). Adding ReasoningTokens here would
	// double-count.

	// Preserve nil when usage exists only in categories without configured
	// pricing, so callers can distinguish "unpriced" from "priced at zero".
	hasMatchingPrice := (usage.InputTokens != nil && cost.InputPricePerMillionTokens != nil) ||
		(usage.OutputTokens != nil && cost.OutputPricePerMillionTokens != nil) ||
		(usage.CacheReadTokens != nil && cost.CacheReadPricePerMillionTokens != nil) ||
		(usage.CacheCreationTokens != nil && cost.CacheWritePricePerMillionTokens != nil)
	if !hasMatchingPrice {
		return nil
	}

	inputMicros := calcCost(usage.InputTokens, cost.InputPricePerMillionTokens)
	outputMicros := calcCost(usage.OutputTokens, cost.OutputPricePerMillionTokens)
	cacheReadMicros := calcCost(usage.CacheReadTokens, cost.CacheReadPricePerMillionTokens)
	cacheWriteMicros := calcCost(usage.CacheCreationTokens, cost.CacheWritePricePerMillionTokens)

	total := inputMicros.
		Add(outputMicros).
		Add(cacheReadMicros).
		Add(cacheWriteMicros)
	rounded := total.Ceil().IntPart()
	return &rounded
}

// calcCost returns the cost in fractional microdollars (millionths of a USD)
// for the given token count at the specified per-million-token price.
func calcCost(tokens *int64, pricePerMillion *decimal.Decimal) decimal.Decimal {
	return decimal.NewFromInt(ptr.NilToEmpty(tokens)).Mul(ptr.NilToEmpty(pricePerMillion))
}
