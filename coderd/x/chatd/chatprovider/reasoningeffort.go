package chatprovider

import (
	"slices"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyazure "charm.land/fantasy/providers/azure"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"

	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/codersdk"
)

func reasoningEffortRank(value string) (int, bool) {
	rank := slices.Index(codersdk.ChatModelReasoningEffortValues(), value)
	return rank, rank >= 0
}

func IsValidReasoningEffort(value string) bool {
	_, ok := reasoningEffortRank(value)
	return ok
}

// ReasoningEffortLessOrEqual reports whether a is lower than or equal
// to b on the global effort scale. Unknown values return false.
func ReasoningEffortLessOrEqual(a, b string) bool {
	aRank, aOK := reasoningEffortRank(a)
	bRank, bOK := reasoningEffortRank(b)
	return aOK && bOK && aRank <= bRank
}

// ResolveReasoningEffort computes the effective reasoning effort for a
// generation. The requested per-turn value wins over the config's default,
// and the result is clamped to the config's max on the global scale. Returns
// nil when the model config has no reasoning effort configured, no usable
// value remains, or the max is unknown.
func ResolveReasoningEffort(
	requested *string,
	config *codersdk.ChatModelReasoningEffortConfig,
) *string {
	if config == nil {
		return nil
	}

	effective := requested
	var rank int
	var ok bool
	if effective != nil {
		rank, ok = reasoningEffortRank(*effective)
	}
	if !ok {
		effective = config.Default
		if effective != nil {
			rank, ok = reasoningEffortRank(*effective)
		}
	}
	if !ok {
		return nil
	}
	if config.Max != nil {
		maxRank, ok := reasoningEffortRank(*config.Max)
		if !ok {
			return nil
		}
		if rank > maxRank {
			return config.Max
		}
	}
	return effective
}

func SelectableReasoningEfforts(
	config *codersdk.ChatModelReasoningEffortConfig,
) []string {
	if config == nil || config.Max == nil {
		return nil
	}
	maxRank, ok := reasoningEffortRank(*config.Max)
	if !ok {
		return nil
	}
	values := codersdk.ChatModelReasoningEffortValues()
	return values[:maxRank+1]
}

func ApplyReasoningEffort(
	model fantasy.LanguageModel,
	options fantasy.ProviderOptions,
	effort *string,
) fantasy.ProviderOptions {
	if effort == nil || model == nil {
		return options
	}
	if options == nil {
		options = fantasy.ProviderOptions{}
	}

	switch NormalizeProvider(model.Provider()) {
	case fantasyopenai.Name, fantasyazure.Name:
		providerEffort := fantasyopenai.ReasoningEffort(*effort)
		switch opts := options[fantasyopenai.Name].(type) {
		case *fantasyopenai.ResponsesProviderOptions:
			opts.ReasoningEffort = &providerEffort
		case *fantasyopenai.ProviderOptions:
			opts.ReasoningEffort = &providerEffort
		default:
			if chatopenai.UsesResponsesOptions(model) {
				options[fantasyopenai.Name] = &fantasyopenai.ResponsesProviderOptions{
					ReasoningEffort: &providerEffort,
				}
				return options
			}
			options[fantasyopenai.Name] = &fantasyopenai.ProviderOptions{
				ReasoningEffort: &providerEffort,
			}
		}
	case fantasyanthropic.Name, fantasybedrock.Name:
		providerEffort := fantasyanthropic.Effort(*effort)
		providerOptions := ensureProviderOptions[fantasyanthropic.ProviderOptions](options, fantasyanthropic.Name)
		providerOptions.Effort = &providerEffort
	case fantasyopenaicompat.Name:
		providerEffort := fantasyopenai.ReasoningEffort(*effort)
		providerOptions := ensureProviderOptions[fantasyopenaicompat.ProviderOptions](options, fantasyopenaicompat.Name)
		providerOptions.ReasoningEffort = &providerEffort
	case fantasyopenrouter.Name:
		providerEffort := fantasyopenrouter.ReasoningEffort(*effort)
		providerOptions := ensureProviderOptions[fantasyopenrouter.ProviderOptions](options, fantasyopenrouter.Name)
		if providerOptions.Reasoning == nil {
			providerOptions.Reasoning = &fantasyopenrouter.ReasoningOptions{}
		}
		providerOptions.Reasoning.Effort = &providerEffort
	case fantasyvercel.Name:
		providerEffort := fantasyvercel.ReasoningEffort(*effort)
		providerOptions := ensureProviderOptions[fantasyvercel.ProviderOptions](options, fantasyvercel.Name)
		if providerOptions.Reasoning == nil {
			providerOptions.Reasoning = &fantasyvercel.ReasoningOptions{}
		}
		providerOptions.Reasoning.Effort = &providerEffort
	}
	return options
}

func ensureProviderOptions[T any, PT interface {
	*T
	fantasy.ProviderOptionsData
}](options fantasy.ProviderOptions, name string) PT {
	providerOptions, _ := options[name].(PT)
	if providerOptions == nil {
		providerOptions = PT(new(T))
		options[name] = providerOptions
	}
	return providerOptions
}
