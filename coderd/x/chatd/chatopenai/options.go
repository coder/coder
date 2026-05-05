package chatopenai

import (
	"slices"
	"strings"

	"charm.land/fantasy"
	fantasyazure "charm.land/fantasy/providers/azure"
	fantasyopenai "charm.land/fantasy/providers/openai"

	"github.com/coder/coder/v2/coderd/x/chatd/chatutil"
	"github.com/coder/coder/v2/codersdk"
)

// ProviderOptionsFromChatConfig converts chat model OpenAI options to fantasy
// provider options used for inference calls.
func ProviderOptionsFromChatConfig(
	model fantasy.LanguageModel,
	options *codersdk.ChatModelOpenAIProviderOptions,
) fantasy.ProviderOptionsData {
	reasoningEffort := ReasoningEffortFromChat(options.ReasoningEffort)
	if UsesResponsesOptions(model) {
		include := EnsureResponseIncludes(IncludeFromChat(options.Include))
		providerOptions := &fantasyopenai.ResponsesProviderOptions{
			Include:           include,
			Instructions:      chatutil.NormalizedStringPointer(options.Instructions),
			Logprobs:          ResponsesLogProbsFromChatConfig(options),
			MaxToolCalls:      options.MaxToolCalls,
			Metadata:          options.Metadata,
			ParallelToolCalls: options.ParallelToolCalls,
			PromptCacheKey:    chatutil.NormalizedStringPointer(options.PromptCacheKey),
			ReasoningEffort:   reasoningEffort,
			ReasoningSummary:  chatutil.NormalizedStringPointer(options.ReasoningSummary),
			SafetyIdentifier:  chatutil.NormalizedStringPointer(options.SafetyIdentifier),
			ServiceTier:       ServiceTierFromChat(options.ServiceTier),
			StrictJSONSchema:  options.StrictJSONSchema,
			Store:             boolPtrOrDefault(options.Store, true),
			TextVerbosity:     TextVerbosityFromChat(options.TextVerbosity),
			User:              chatutil.NormalizedStringPointer(options.User),
		}
		return providerOptions
	}

	return &fantasyopenai.ProviderOptions{
		LogitBias:           options.LogitBias,
		LogProbs:            options.LogProbs,
		TopLogProbs:         options.TopLogProbs,
		ParallelToolCalls:   options.ParallelToolCalls,
		User:                chatutil.NormalizedStringPointer(options.User),
		ReasoningEffort:     reasoningEffort,
		MaxCompletionTokens: options.MaxCompletionTokens,
		TextVerbosity:       chatutil.NormalizedStringPointer(options.TextVerbosity),
		Prediction:          options.Prediction,
		Store:               boolPtrOrDefault(options.Store, true),
		Metadata:            options.Metadata,
		PromptCacheKey:      chatutil.NormalizedStringPointer(options.PromptCacheKey),
		SafetyIdentifier:    chatutil.NormalizedStringPointer(options.SafetyIdentifier),
		ServiceTier:         chatutil.NormalizedStringPointer(options.ServiceTier),
		StructuredOutputs:   options.StructuredOutputs,
	}
}

// TextVerbosityFromChat normalizes chat-config text verbosity values for
// OpenAI and returns the canonical provider verbosity value.
func TextVerbosityFromChat(value *string) *fantasyopenai.TextVerbosity {
	if value == nil {
		return nil
	}

	normalized := strings.ToLower(strings.TrimSpace(*value))
	if normalized == "" {
		return nil
	}

	verbosity := chatutil.NormalizedEnumValue(
		normalized,
		string(fantasyopenai.TextVerbosityLow),
		string(fantasyopenai.TextVerbosityMedium),
		string(fantasyopenai.TextVerbosityHigh),
	)
	if verbosity == nil {
		return nil
	}
	valueCopy := fantasyopenai.TextVerbosity(*verbosity)
	return &valueCopy
}

// IncludeFromChat converts chat-config include values to OpenAI Responses
// include values and ignores unsupported entries.
func IncludeFromChat(values []string) []fantasyopenai.IncludeType {
	if values == nil {
		return nil
	}

	result := make([]fantasyopenai.IncludeType, 0, len(values))
	for _, value := range values {
		switch strings.TrimSpace(value) {
		case string(fantasyopenai.IncludeReasoningEncryptedContent):
			result = append(result, fantasyopenai.IncludeReasoningEncryptedContent)
		case string(fantasyopenai.IncludeFileSearchCallResults):
			result = append(result, fantasyopenai.IncludeFileSearchCallResults)
		case string(fantasyopenai.IncludeMessageOutputTextLogprobs):
			result = append(result, fantasyopenai.IncludeMessageOutputTextLogprobs)
		}
	}
	return result
}

// EnsureResponseIncludes adds the OpenAI encrypted reasoning include required
// for Responses API reasoning continuity when it is not already present.
func EnsureResponseIncludes(
	values []fantasyopenai.IncludeType,
) []fantasyopenai.IncludeType {
	const required = fantasyopenai.IncludeReasoningEncryptedContent

	if slices.Contains(values, required) {
		return values
	}
	return append(values, required)
}

// UsesResponsesOptions reports whether the model should use OpenAI Responses
// API provider options.
func UsesResponsesOptions(model fantasy.LanguageModel) bool {
	if model == nil {
		return false
	}
	switch model.Provider() {
	case fantasyopenai.Name, fantasyazure.Name:
		return fantasyopenai.IsResponsesModel(model.Model())
	default:
		return false
	}
}

// ReasoningEffortFromChat normalizes chat-config reasoning effort values for
// OpenAI and returns the canonical provider effort value.
func ReasoningEffortFromChat(value *string) *fantasyopenai.ReasoningEffort {
	if value == nil {
		return nil
	}

	normalized := strings.ToLower(strings.TrimSpace(*value))
	if normalized == "" {
		return nil
	}

	effort := chatutil.NormalizedEnumValue(
		normalized,
		string(fantasyopenai.ReasoningEffortMinimal),
		string(fantasyopenai.ReasoningEffortLow),
		string(fantasyopenai.ReasoningEffortMedium),
		string(fantasyopenai.ReasoningEffortHigh),
		string(fantasyopenai.ReasoningEffortXHigh),
	)
	if effort == nil {
		return nil
	}
	valueCopy := fantasyopenai.ReasoningEffort(*effort)
	return &valueCopy
}

// ServiceTierFromChat normalizes chat-config service tier values for OpenAI
// Responses API and returns the canonical provider service tier value.
func ServiceTierFromChat(value *string) *fantasyopenai.ServiceTier {
	normalized := chatutil.NormalizedStringPointer(value)
	if normalized == nil {
		return nil
	}
	switch strings.ToLower(*normalized) {
	case string(fantasyopenai.ServiceTierAuto):
		serviceTier := fantasyopenai.ServiceTierAuto
		return &serviceTier
	case string(fantasyopenai.ServiceTierFlex):
		serviceTier := fantasyopenai.ServiceTierFlex
		return &serviceTier
	case string(fantasyopenai.ServiceTierPriority):
		serviceTier := fantasyopenai.ServiceTierPriority
		return &serviceTier
	default:
		return nil
	}
}

// ResponsesLogProbsFromChatConfig maps chat-config log probability options to the
// value expected by OpenAI Responses provider options.
func ResponsesLogProbsFromChatConfig(
	options *codersdk.ChatModelOpenAIProviderOptions,
) any {
	if options == nil {
		return nil
	}
	if options.TopLogProbs != nil {
		return *options.TopLogProbs
	}
	if options.LogProbs != nil {
		return *options.LogProbs
	}
	return nil
}

// IsReasoningModel reports whether a model ID follows OpenAI reasoning model
// naming conventions.
func IsReasoningModel(modelID string) bool {
	if len(modelID) < 2 || modelID[0] != 'o' {
		return false
	}

	index := 1
	for index < len(modelID) && modelID[index] >= '0' && modelID[index] <= '9' {
		index++
	}
	if index == 1 {
		return false
	}

	if index == len(modelID) {
		return true
	}
	return modelID[index] == '-' || modelID[index] == '.'
}

func boolPtrOrDefault(value *bool, def bool) *bool {
	if value != nil {
		return value
	}
	return &def
}
