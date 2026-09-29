package chatprovider

import (
	"slices"
	"strconv"
	"strings"

	fantasyopenai "charm.land/fantasy/providers/openai"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/codersdk"
)

// ExactReasoningEfforts lists configured efforts the provider adapter can
// transmit without dropping, translating, or clamping the requested value.
// Provider rejection remains an error; this is not a remote capability probe.
func ExactReasoningEfforts(provider, model string, config codersdk.ChatModelCallConfig) []string {
	var supported []string
	for _, effort := range SelectableReasoningEfforts(config.ReasoningEffort) {
		if exactProviderEffort(provider, model, config, effort) {
			supported = append(supported, effort)
		}
	}
	return supported
}

// ResolveExactReasoningEffort resolves an explicit choice or configured default
// without falling back. A nil result means the provider controls the default.
func ResolveExactReasoningEffort(provider, model string, config codersdk.ChatModelCallConfig, requested *string) (*string, error) {
	effort := requested
	if effort == nil && config.ReasoningEffort != nil {
		effort = config.ReasoningEffort.Default
	}
	if effort == nil {
		return nil, nil //nolint:nilnil // Unconfigured effort is valid and must stay absent.
	}
	supported := ExactReasoningEfforts(provider, model, config)
	if !slices.Contains(supported, *effort) {
		return nil, xerrors.Errorf("reasoning effort %q cannot be applied exactly to %s/%s; supported efforts: %s", *effort, provider, model, strings.Join(supported, ", "))
	}
	return new(*effort), nil
}

func exactProviderEffort(provider, model string, config codersdk.ChatModelCallConfig, effort string) bool {
	switch NormalizeProvider(provider) {
	case "openai", "azure":
		openAIConfig := effectiveOpenAIConfig(&config)
		var override *bool
		if openAIConfig != nil {
			override = openAIConfig.ReasoningModel
		}
		if NormalizeProvider(provider) == "azure" {
			override = nil // Azure does not accept the OpenAI construction override.
		}
		reasoning := fantasyopenai.IsResponsesReasoningModel(model)
		responsesOverride := openAIResponsesAPIOverride(openAIConfig)
		if NormalizeProvider(provider) == "azure" {
			responsesOverride = nil
		}
		if !chatopenai.UsesResponsesAPI(model, responsesOverride) {
			// Match the pinned provider's chat-completions classification.
			reasoning = strings.HasPrefix(model, "o1") || strings.Contains(model, "-o1") ||
				strings.HasPrefix(model, "o3") || strings.Contains(model, "-o3") ||
				strings.HasPrefix(model, "o4") || strings.Contains(model, "-o4") ||
				strings.HasPrefix(model, "oss") || strings.Contains(model, "-oss") ||
				strings.Contains(strings.ToLower(model), "gpt-5")
		}
		if override != nil {
			reasoning = *override
		}
		return reasoning && openAIReasoningEffort(model, effort) == effort
	case "anthropic":
		return exactAnthropicEffort(model, effort)
	case "google":
		if config.ProviderOptions != nil && config.ProviderOptions.Google != nil &&
			config.ProviderOptions.Google.ThinkingConfig != nil &&
			config.ProviderOptions.Google.ThinkingConfig.ThinkingBudget != nil {
			return false // A budget overrides the selected effort.
		}
		levels := googleSupportedThinkingLevels(model)
		if len(levels) == 0 {
			return false
		}
		level := strings.ToLower(clampGoogleThinkingLevel(googleThinkingLevel(effort), levels))
		return level == effort
	case "openai-compat":
		if mapped, ok := googleCompatReasoningEffort(model, effort); ok {
			return mapped == effort
		}
		return true // The adapter sends the explicit value unchanged.
	case "openrouter", "vercel":
		return true // Provider rejection is surfaced; no local substitution occurs.
	default:
		// Bedrock selects different underlying adapters by model identity. Until
		// that adapter is known, do not advertise an exact effort guarantee.
		return false
	}
}

// Older Anthropic models translate effort into token budgets; exact selection
// requires a model whose wire protocol accepts the effort itself.
func exactAnthropicEffort(model, effort string) bool {
	normalized := strings.ToLower(model)
	_, version, ok := strings.Cut(normalized, "claude-")
	if !ok || anthropicReasoningEffort(model, effort) != effort {
		return false
	}
	version, _, _ = strings.Cut(version, "@")
	var major, minor int
	found := false
	parts := strings.Split(version, "-")
	for i, part := range parts {
		if len(part) > 2 {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		major = n
		if i+1 < len(parts) && len(parts[i+1]) <= 2 {
			minor, _ = strconv.Atoi(parts[i+1])
		}
		found = true
		break
	}
	if !found || major < 4 || (major == 4 && minor < 5) {
		return false
	}
	// The pinned adapter sends effort on 4.5 only for Opus; other 4.5
	// models convert it into a legacy token budget.
	if major == 4 && minor == 5 && !strings.Contains(normalized, "opus") {
		return false
	}
	switch effort {
	case "minimal":
		return false
	case "none":
		return major >= 5 && !strings.Contains(normalized, "claude-mythos") && !strings.Contains(normalized, "claude-fable")
	case "xhigh":
		return major > 4 || (major == 4 && minor >= 7)
	case "max":
		return major > 4 || (major == 4 && minor >= 6)
	default:
		return true
	}
}
