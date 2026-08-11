package chatopenai

import (
	"slices"
	"strings"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"

	"github.com/coder/coder/v2/coderd/x/chatd/chatutil"
	"github.com/coder/coder/v2/codersdk"
)

// ProviderOptionsFromChatConfig converts chat model OpenAI options to fantasy
// provider options used for inference calls.
func ProviderOptionsFromChatConfig(
	transport Transport,
	options *codersdk.ChatModelOpenAIProviderOptions,
) fantasy.ProviderOptionsData {
	if transport.UsesResponses() {
		include := EnsureResponseIncludes(IncludeFromChat(options.Include))
		providerOptions := &fantasyopenai.ResponsesProviderOptions{
			Include:           include,
			Instructions:      chatutil.NormalizedStringPointer(options.Instructions),
			Logprobs:          ResponsesLogProbsFromChatConfig(options),
			MaxToolCalls:      options.MaxToolCalls,
			Metadata:          options.Metadata,
			ParallelToolCalls: options.ParallelToolCalls,
			PromptCacheKey:    chatutil.NormalizedStringPointer(options.PromptCacheKey),
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

// ServiceTierFromChat normalizes chat-config service tier values for the
// OpenAI Responses API. It maps every tier the codersdk enum advertises, not
// only the ones fantasy declares constants for, because fantasy forwards the
// value to the API unchanged.
func ServiceTierFromChat(value *string) *fantasyopenai.ServiceTier {
	normalized := chatutil.NormalizedStringPointer(value)
	if normalized == nil {
		return nil
	}
	tier := chatutil.NormalizedEnumValue(
		strings.ToLower(*normalized),
		string(fantasyopenai.ServiceTierAuto),
		"default",
		string(fantasyopenai.ServiceTierFlex),
		"scale",
		string(fantasyopenai.ServiceTierPriority),
	)
	if tier == nil {
		return nil
	}
	serviceTier := fantasyopenai.ServiceTier(*tier)
	return &serviceTier
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
