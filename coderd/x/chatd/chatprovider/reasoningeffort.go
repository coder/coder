package chatprovider

import (
	"strings"

	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyazure "charm.land/fantasy/providers/azure"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"

	"github.com/coder/coder/v2/codersdk"
)

// reasoningEffortOrder is the global reasoning effort scale used for
// clamping and comparison. Each provider supports a contiguous subset.
var reasoningEffortOrder = []string{
	"none",
	"minimal",
	"low",
	"medium",
	"high",
	"xhigh",
	"max",
}

// ReasoningEffortRank returns the position of value on the global
// effort scale. Unknown values return ok=false.
func ReasoningEffortRank(value string) (int, bool) {
	for i, v := range reasoningEffortOrder {
		if v == value {
			return i, true
		}
	}
	return 0, false
}

// NormalizeGlobalReasoningEffort lowercases and trims value and
// returns it when it is on the global effort scale, or nil otherwise.
func NormalizeGlobalReasoningEffort(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*value))
	if _, ok := ReasoningEffortRank(normalized); !ok {
		return nil
	}
	return &normalized
}

// SupportedReasoningEfforts returns the provider's runtime-supported
// effort values in ascending global order. Azure shares OpenAI's set
// and Bedrock shares Anthropic's. Providers without reasoning effort
// support return nil.
func SupportedReasoningEfforts(provider string) []string {
	switch NormalizeProvider(provider) {
	case fantasyopenai.Name, fantasyazure.Name, fantasyopenaicompat.Name:
		return []string{"minimal", "low", "medium", "high", "xhigh"}
	case fantasyanthropic.Name, fantasybedrock.Name:
		return []string{"low", "medium", "high", "xhigh", "max"}
	case fantasyopenrouter.Name:
		return []string{"low", "medium", "high"}
	case fantasyvercel.Name:
		return []string{"none", "minimal", "low", "medium", "high", "xhigh"}
	default:
		return nil
	}
}

// ResolveReasoningEffort computes the effective reasoning effort for a
// generation. The requested per-turn value wins over the config's
// default; the result is clamped to the config's max on the global
// scale and then snapped into the provider's supported set: the
// largest supported value not exceeding the clamped value, or the
// provider minimum when the value is below it. Returns nil when the
// model config has no reasoning effort configured, when no usable
// value remains, or when the provider does not support reasoning
// effort.
func ResolveReasoningEffort(
	provider string,
	requested *string,
	config *codersdk.ChatModelReasoningEffortConfig,
) *string {
	if config == nil {
		return nil
	}

	effective := NormalizeGlobalReasoningEffort(requested)
	if effective == nil {
		effective = NormalizeGlobalReasoningEffort(config.Default)
	}
	if effective == nil {
		return nil
	}
	rank, _ := ReasoningEffortRank(*effective)

	if maxEffort := NormalizeGlobalReasoningEffort(config.Max); maxEffort != nil {
		if maxRank, _ := ReasoningEffortRank(*maxEffort); rank > maxRank {
			rank = maxRank
		}
	}

	supported := SupportedReasoningEfforts(provider)
	if len(supported) == 0 {
		return nil
	}

	// Snap to the largest supported value not exceeding the effective
	// value. Values below the provider minimum clamp up to it so
	// reasoning is not silently disabled.
	result := supported[0]
	for _, candidate := range supported {
		candidateRank, _ := ReasoningEffortRank(candidate)
		if candidateRank > rank {
			break
		}
		result = candidate
	}
	return &result
}
