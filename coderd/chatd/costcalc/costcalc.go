package costcalc

import (
	"math"

	"github.com/coder/coder/v2/codersdk"
)

// CalculateTotalCostMicros calculates the total chat usage cost in micros.
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

	inputMicros := math.Round(float64(derefInt64(usage.InputTokens)) *
		derefFloat64(cost.InputPricePerMillionTokens))
	outputMicros := math.Round(float64(derefInt64(usage.OutputTokens)+derefInt64(usage.ReasoningTokens)) *
		derefFloat64(cost.OutputPricePerMillionTokens))
	cacheReadMicros := math.Round(float64(derefInt64(usage.CacheReadTokens)) *
		derefFloat64(cost.CacheReadPricePerMillionTokens))
	cacheWriteMicros := math.Round(float64(derefInt64(usage.CacheCreationTokens)) *
		derefFloat64(cost.CacheWritePricePerMillionTokens))

	total := int64(inputMicros + outputMicros + cacheReadMicros + cacheWriteMicros)
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
