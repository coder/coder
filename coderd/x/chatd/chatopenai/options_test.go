package chatopenai_test

import (
	"context"
	"testing"

	"charm.land/fantasy"
	fantasyazure "charm.land/fantasy/providers/azure"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/codersdk"
)

func TestProviderOptionsFromChatConfigLegacy(t *testing.T) {
	t.Parallel()

	store := false
	logProbs := true
	topLogProbs := int64(3)
	parallelToolCalls := true
	maxCompletionTokens := int64(4096)
	structuredOutputs := true
	options := &codersdk.ChatModelOpenAIProviderOptions{
		LogitBias: map[string]int64{
			"50256": -10,
		},
		LogProbs:            &logProbs,
		TopLogProbs:         &topLogProbs,
		ParallelToolCalls:   &parallelToolCalls,
		User:                ptr(" user-1 "),
		ReasoningEffort:     ptr(" HIGH "),
		MaxCompletionTokens: &maxCompletionTokens,
		TextVerbosity:       ptr(" High "),
		Prediction: map[string]any{
			"type": "content",
		},
		Store:             &store,
		Metadata:          map[string]any{"feature": "chat"},
		PromptCacheKey:    ptr(" cache-key "),
		SafetyIdentifier:  ptr(" safety-id "),
		ServiceTier:       ptr(" priority "),
		StructuredOutputs: &structuredOutputs,
	}

	got := chatopenai.ProviderOptionsFromChatConfig(
		fakeLanguageModel{provider: fantasyopenai.Name, model: "gpt-3.5-turbo-instruct"},
		options,
	)

	providerOptions, ok := got.(*fantasyopenai.ProviderOptions)
	require.True(t, ok)
	require.Equal(t, options.LogitBias, providerOptions.LogitBias)
	require.Same(t, options.LogProbs, providerOptions.LogProbs)
	require.Same(t, options.TopLogProbs, providerOptions.TopLogProbs)
	require.Same(t, options.ParallelToolCalls, providerOptions.ParallelToolCalls)
	require.Equal(t, "user-1", requireStringPointerValue(t, providerOptions.User))
	require.Equal(t, fantasyopenai.ReasoningEffortHigh, requireReasoningEffortPointerValue(t, providerOptions.ReasoningEffort))
	require.Same(t, options.MaxCompletionTokens, providerOptions.MaxCompletionTokens)
	require.Equal(t, "High", requireStringPointerValue(t, providerOptions.TextVerbosity))
	require.Equal(t, options.Prediction, providerOptions.Prediction)
	require.Same(t, options.Store, providerOptions.Store)
	require.Equal(t, false, requireBoolPointerValue(t, providerOptions.Store))
	require.Equal(t, options.Metadata, providerOptions.Metadata)
	require.Equal(t, "cache-key", requireStringPointerValue(t, providerOptions.PromptCacheKey))
	require.Equal(t, "safety-id", requireStringPointerValue(t, providerOptions.SafetyIdentifier))
	require.Equal(t, "priority", requireStringPointerValue(t, providerOptions.ServiceTier))
	require.Same(t, options.StructuredOutputs, providerOptions.StructuredOutputs)
}

func TestProviderOptionsFromChatConfigResponses(t *testing.T) {
	t.Parallel()

	topLogProbs := int64(5)
	maxToolCalls := int64(8)
	parallelToolCalls := false
	strictJSONSchema := true
	options := &codersdk.ChatModelOpenAIProviderOptions{
		Include: []string{
			string(fantasyopenai.IncludeFileSearchCallResults),
			"unsupported",
		},
		Instructions:      ptr(" instructions "),
		LogProbs:          ptr(true),
		TopLogProbs:       &topLogProbs,
		MaxToolCalls:      &maxToolCalls,
		Metadata:          map[string]any{"scope": "unit"},
		ParallelToolCalls: &parallelToolCalls,
		PromptCacheKey:    ptr(" prompt-cache "),
		ReasoningEffort:   ptr(" minimal "),
		ReasoningSummary:  ptr(" auto "),
		SafetyIdentifier:  ptr(" safety "),
		ServiceTier:       ptr(" FLEX "),
		StrictJSONSchema:  &strictJSONSchema,
		TextVerbosity:     ptr(" MEDIUM "),
		User:              ptr(" user-2 "),
	}

	got := chatopenai.ProviderOptionsFromChatConfig(
		fakeLanguageModel{provider: fantasyopenai.Name, model: "gpt-4.1"},
		options,
	)

	providerOptions, ok := got.(*fantasyopenai.ResponsesProviderOptions)
	require.True(t, ok)
	require.Equal(t, []fantasyopenai.IncludeType{
		fantasyopenai.IncludeFileSearchCallResults,
		fantasyopenai.IncludeReasoningEncryptedContent,
	}, providerOptions.Include)
	require.Equal(t, "instructions", requireStringPointerValue(t, providerOptions.Instructions))
	require.Equal(t, int64(5), providerOptions.Logprobs)
	require.Same(t, options.MaxToolCalls, providerOptions.MaxToolCalls)
	require.Equal(t, options.Metadata, providerOptions.Metadata)
	require.Same(t, options.ParallelToolCalls, providerOptions.ParallelToolCalls)
	require.Equal(t, "prompt-cache", requireStringPointerValue(t, providerOptions.PromptCacheKey))
	require.Equal(t, fantasyopenai.ReasoningEffortMinimal, requireReasoningEffortPointerValue(t, providerOptions.ReasoningEffort))
	require.Equal(t, "auto", requireStringPointerValue(t, providerOptions.ReasoningSummary))
	require.Equal(t, "safety", requireStringPointerValue(t, providerOptions.SafetyIdentifier))
	require.Equal(t, fantasyopenai.ServiceTierFlex, requireServiceTierPointerValue(t, providerOptions.ServiceTier))
	require.Same(t, options.StrictJSONSchema, providerOptions.StrictJSONSchema)
	require.NotNil(t, providerOptions.Store)
	require.True(t, *providerOptions.Store)
	require.Equal(t, fantasyopenai.TextVerbosityMedium, requireTextVerbosityPointerValue(t, providerOptions.TextVerbosity))
	require.Equal(t, "user-2", requireStringPointerValue(t, providerOptions.User))
}

func TestTextVerbosityFromChat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value *string
		want  *fantasyopenai.TextVerbosity
	}{
		{name: "Nil"},
		{name: "Empty", value: ptr("  ")},
		{name: "Low", value: ptr(" low "), want: ptr(fantasyopenai.TextVerbosityLow)},
		{name: "MediumCase", value: ptr(" MEDIUM "), want: ptr(fantasyopenai.TextVerbosityMedium)},
		{name: "High", value: ptr("high"), want: ptr(fantasyopenai.TextVerbosityHigh)},
		{name: "Invalid", value: ptr("verbose")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatopenai.TextVerbosityFromChat(tt.value)
			if tt.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tt.want, *got)
		})
	}
}

func TestIncludeFromChat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
		want   []fantasyopenai.IncludeType
	}{
		{name: "Nil"},
		{name: "Empty", values: []string{}, want: []fantasyopenai.IncludeType{}},
		{
			name: "ValidAndInvalid",
			values: []string{
				" " + string(fantasyopenai.IncludeReasoningEncryptedContent) + " ",
				string(fantasyopenai.IncludeFileSearchCallResults),
				"unsupported",
				string(fantasyopenai.IncludeMessageOutputTextLogprobs),
			},
			want: []fantasyopenai.IncludeType{
				fantasyopenai.IncludeReasoningEncryptedContent,
				fantasyopenai.IncludeFileSearchCallResults,
				fantasyopenai.IncludeMessageOutputTextLogprobs,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatopenai.IncludeFromChat(tt.values)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestEnsureResponseIncludes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []fantasyopenai.IncludeType
		want   []fantasyopenai.IncludeType
	}{
		{
			name: "NilAddsRequired",
			want: []fantasyopenai.IncludeType{fantasyopenai.IncludeReasoningEncryptedContent},
		},
		{
			name:   "EmptyAddsRequired",
			values: []fantasyopenai.IncludeType{},
			want:   []fantasyopenai.IncludeType{fantasyopenai.IncludeReasoningEncryptedContent},
		},
		{
			name: "AddsRequiredAfterExistingValues",
			values: []fantasyopenai.IncludeType{
				fantasyopenai.IncludeFileSearchCallResults,
			},
			want: []fantasyopenai.IncludeType{
				fantasyopenai.IncludeFileSearchCallResults,
				fantasyopenai.IncludeReasoningEncryptedContent,
			},
		},
		{
			name: "DoesNotDuplicateRequired",
			values: []fantasyopenai.IncludeType{
				fantasyopenai.IncludeReasoningEncryptedContent,
				fantasyopenai.IncludeFileSearchCallResults,
			},
			want: []fantasyopenai.IncludeType{
				fantasyopenai.IncludeReasoningEncryptedContent,
				fantasyopenai.IncludeFileSearchCallResults,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatopenai.EnsureResponseIncludes(tt.values)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestUsesResponsesOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model fantasy.LanguageModel
		want  bool
	}{
		{name: "Nil"},
		{
			name:  "OpenAIResponsesModel",
			model: fakeLanguageModel{provider: fantasyopenai.Name, model: "gpt-4.1"},
			want:  true,
		},
		{
			name:  "AzureResponsesModel",
			model: fakeLanguageModel{provider: fantasyazure.Name, model: "gpt-4.1"},
			want:  true,
		},
		{
			name:  "OpenAINonResponsesModel",
			model: fakeLanguageModel{provider: fantasyopenai.Name, model: "gpt-3.5-turbo-instruct"},
		},
		{
			name:  "NonOpenAIProvider",
			model: fakeLanguageModel{provider: "other", model: "gpt-4.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatopenai.UsesResponsesOptions(tt.model)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestReasoningEffortFromChat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value *string
		want  *fantasyopenai.ReasoningEffort
	}{
		{name: "Nil"},
		{name: "Empty", value: ptr("  ")},
		{name: "Minimal", value: ptr(" minimal "), want: ptr(fantasyopenai.ReasoningEffortMinimal)},
		{name: "LowCase", value: ptr(" LOW "), want: ptr(fantasyopenai.ReasoningEffortLow)},
		{name: "Medium", value: ptr("medium"), want: ptr(fantasyopenai.ReasoningEffortMedium)},
		{name: "High", value: ptr("high"), want: ptr(fantasyopenai.ReasoningEffortHigh)},
		{name: "XHigh", value: ptr("xhigh"), want: ptr(fantasyopenai.ReasoningEffortXHigh)},
		{name: "NoneUnsupported", value: ptr("none")},
		{name: "Invalid", value: ptr("max")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatopenai.ReasoningEffortFromChat(tt.value)
			if tt.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tt.want, *got)
		})
	}
}

func TestServiceTierFromChat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value *string
		want  *fantasyopenai.ServiceTier
	}{
		{name: "Nil"},
		{name: "Empty", value: ptr("  ")},
		{name: "Auto", value: ptr(" auto "), want: ptr(fantasyopenai.ServiceTierAuto)},
		{name: "FlexCase", value: ptr(" FLEX "), want: ptr(fantasyopenai.ServiceTierFlex)},
		{name: "Priority", value: ptr("priority"), want: ptr(fantasyopenai.ServiceTierPriority)},
		{name: "DefaultUnsupported", value: ptr("default")},
		{name: "Invalid", value: ptr("fast")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatopenai.ServiceTierFromChat(tt.value)
			if tt.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tt.want, *got)
		})
	}
}

func TestResponsesLogProbsFromChatConfig(t *testing.T) {
	t.Parallel()

	logProbs := true
	topLogProbs := int64(4)
	tests := []struct {
		name    string
		options *codersdk.ChatModelOpenAIProviderOptions
		want    any
	}{
		{name: "Nil"},
		{
			name:    "Empty",
			options: &codersdk.ChatModelOpenAIProviderOptions{},
		},
		{
			name: "LogProbs",
			options: &codersdk.ChatModelOpenAIProviderOptions{
				LogProbs: &logProbs,
			},
			want: true,
		},
		{
			name: "TopLogProbs",
			options: &codersdk.ChatModelOpenAIProviderOptions{
				TopLogProbs: &topLogProbs,
			},
			want: int64(4),
		},
		{
			name: "TopLogProbsPrecedence",
			options: &codersdk.ChatModelOpenAIProviderOptions{
				LogProbs:    &logProbs,
				TopLogProbs: &topLogProbs,
			},
			want: int64(4),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatopenai.ResponsesLogProbsFromChatConfig(tt.options)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIsReasoningModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model string
		want  bool
	}{
		{model: ""},
		{model: "o"},
		{model: "o1", want: true},
		{model: "o1-mini", want: true},
		{model: "o3.5", want: true},
		{model: "o10-preview", want: true},
		{model: "oabc"},
		{model: "ox"},
		{model: "o1preview"},
		{model: "gpt-5"},
		{model: "O1"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			t.Parallel()

			got := chatopenai.IsReasoningModel(tt.model)
			require.Equal(t, tt.want, got)
		})
	}
}

func requireStringPointerValue(t *testing.T, value *string) string {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func requireBoolPointerValue(t *testing.T, value *bool) bool {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func requireReasoningEffortPointerValue(
	t *testing.T,
	value *fantasyopenai.ReasoningEffort,
) fantasyopenai.ReasoningEffort {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func requireServiceTierPointerValue(
	t *testing.T,
	value *fantasyopenai.ServiceTier,
) fantasyopenai.ServiceTier {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func requireTextVerbosityPointerValue(
	t *testing.T,
	value *fantasyopenai.TextVerbosity,
) fantasyopenai.TextVerbosity {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func ptr[T any](value T) *T {
	return &value
}

type fakeLanguageModel struct {
	provider string
	model    string
}

func (fakeLanguageModel) Generate(context.Context, fantasy.Call) (*fantasy.Response, error) {
	panic("not implemented")
}

func (fakeLanguageModel) Stream(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
	panic("not implemented")
}

func (fakeLanguageModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	panic("not implemented")
}

func (fakeLanguageModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	panic("not implemented")
}

func (f fakeLanguageModel) Provider() string {
	return f.provider
}

func (f fakeLanguageModel) Model() string {
	return f.model
}
