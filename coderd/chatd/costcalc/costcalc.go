package costcalc

import (
	"math"

	"github.com/coder/coder/v2/codersdk"
)

// CalculateTotalCostMicros computes the total cost of a chat message in
// micros (millionths of a dollar) using the configured model pricing.
//
// All cost components are summed at full float64 precision before a single
// math.Round at the end. Messages with a true cost below 0.5 micros
// (~$0.0000005) will still round to zero. This is accepted behavior for the
// int64 micros storage format. For context, this threshold corresponds to
// roughly 50 tokens on a model priced at $0.01/million tokens, which is well
// below typical message sizes.
//
// Returns nil when pricing is not configured, allowing callers to distinguish
// "zero cost" from "unpriced".
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

	inputMicros := float64(derefInt64(usage.InputTokens)) *
		derefFloat64(cost.InputPricePerMillionTokens)
	outputMicros := float64(derefInt64(usage.OutputTokens)+derefInt64(usage.ReasoningTokens)) *
		derefFloat64(cost.OutputPricePerMillionTokens)
	cacheReadMicros := float64(derefInt64(usage.CacheReadTokens)) *
		derefFloat64(cost.CacheReadPricePerMillionTokens)
	cacheWriteMicros := float64(derefInt64(usage.CacheCreationTokens)) *
		derefFloat64(cost.CacheWritePricePerMillionTokens)

	total := int64(math.Round(inputMicros + outputMicros + cacheReadMicros + cacheWriteMicros))
	return &total
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}

	return *v
}

func derefFloat64(v *float64) float64 {
	if v == nil {
		return 0
	}

	return *v
}
