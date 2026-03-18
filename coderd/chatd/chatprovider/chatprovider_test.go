package chatprovider_test

import (
	"testing"

	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

func TestReasoningEffortFromChat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider string
		input    *string
		want     *string
	}{
		{
			name:     "OpenAICaseInsensitive",
			provider: "openai",
			input:    stringPtr(" HIGH "),
			want:     stringPtr(string(fantasyopenai.ReasoningEffortHigh)),
		},
		{
			name:     "AnthropicEffort",
			provider: "anthropic",
			input:    stringPtr("max"),
			want:     stringPtr(string(fantasyanthropic.EffortMax)),
		},
		{
			name:     "OpenRouterEffort",
			provider: "openrouter",
			input:    stringPtr("medium"),
			want:     stringPtr(string(fantasyopenrouter.ReasoningEffortMedium)),
		},
		{
			name:     "VercelEffort",
			provider: "vercel",
			input:    stringPtr("xhigh"),
			want:     stringPtr(string(fantasyvercel.ReasoningEffortXHigh)),
		},
		{
			name:     "LiteLLMEffort",
			provider: "litellm",
			input:    stringPtr("high"),
			want:     stringPtr(string(fantasyopenai.ReasoningEffortHigh)),
		},
		{
			name:     "InvalidEffortReturnsNil",
			provider: "openai",
			input:    stringPtr("unknown"),
			want:     nil,
		},
		{
			name:     "UnsupportedProviderReturnsNil",
			provider: "bedrock",
			input:    stringPtr("high"),
			want:     nil,
		},
		{
			name:     "NilInputReturnsNil",
			provider: "openai",
			input:    nil,
			want:     nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatprovider.ReasoningEffortFromChat(tt.provider, tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMergeMissingProviderOptions_OpenRouterNested(t *testing.T) {
	t.Parallel()

	options := &codersdk.ChatModelProviderOptions{
		OpenRouter: &codersdk.ChatModelOpenRouterProviderOptions{
			Reasoning: &codersdk.ChatModelReasoningOptions{
				Enabled: boolPtr(true),
			},
			Provider: &codersdk.ChatModelOpenRouterProvider{
				Order: []string{"openai"},
			},
		},
	}
	defaults := &codersdk.ChatModelProviderOptions{
		OpenRouter: &codersdk.ChatModelOpenRouterProviderOptions{
			Reasoning: &codersdk.ChatModelReasoningOptions{
				Enabled:   boolPtr(false),
				Exclude:   boolPtr(true),
				MaxTokens: int64Ptr(123),
				Effort:    stringPtr("high"),
			},
			IncludeUsage: boolPtr(true),
			Provider: &codersdk.ChatModelOpenRouterProvider{
				Order:             []string{"anthropic"},
				AllowFallbacks:    boolPtr(true),
				RequireParameters: boolPtr(false),
				DataCollection:    stringPtr("allow"),
				Only:              []string{"openai"},
				Ignore:            []string{"foo"},
				Quantizations:     []string{"int8"},
				Sort:              stringPtr("latency"),
			},
		},
	}

	chatprovider.MergeMissingProviderOptions(&options, defaults)

	require.NotNil(t, options)
	require.NotNil(t, options.OpenRouter)
	require.NotNil(t, options.OpenRouter.Reasoning)
	require.True(t, *options.OpenRouter.Reasoning.Enabled)
	require.Equal(t, true, *options.OpenRouter.Reasoning.Exclude)
	require.EqualValues(t, 123, *options.OpenRouter.Reasoning.MaxTokens)
	require.Equal(t, "high", *options.OpenRouter.Reasoning.Effort)
	require.NotNil(t, options.OpenRouter.IncludeUsage)
	require.True(t, *options.OpenRouter.IncludeUsage)

	require.NotNil(t, options.OpenRouter.Provider)
	require.Equal(t, []string{"openai"}, options.OpenRouter.Provider.Order)
	require.NotNil(t, options.OpenRouter.Provider.AllowFallbacks)
	require.True(t, *options.OpenRouter.Provider.AllowFallbacks)
	require.NotNil(t, options.OpenRouter.Provider.RequireParameters)
	require.False(t, *options.OpenRouter.Provider.RequireParameters)
	require.Equal(t, "allow", *options.OpenRouter.Provider.DataCollection)
	require.Equal(t, []string{"openai"}, options.OpenRouter.Provider.Only)
	require.Equal(t, []string{"foo"}, options.OpenRouter.Provider.Ignore)
	require.Equal(t, []string{"int8"}, options.OpenRouter.Provider.Quantizations)
	require.Equal(t, "latency", *options.OpenRouter.Provider.Sort)
}

func TestNormalizeProvider_LiteLLM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"Exact", "litellm", "litellm"},
		{"UpperCase", "LITELLM", "litellm"},
		{"MixedCase", "LiteLLM", "litellm"},
		{"Padded", "  litellm  ", "litellm"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chatprovider.NormalizeProvider(tt.input))
		})
	}
}

func TestSupportedProviders_IncludesLiteLLM(t *testing.T) {
	t.Parallel()

	providers := chatprovider.SupportedProviders()
	require.Contains(t, providers, "litellm")
}

func TestMergeMissingProviderOptions_LiteLLM(t *testing.T) {
	t.Parallel()

	options := &codersdk.ChatModelProviderOptions{
		LiteLLM: &codersdk.ChatModelLiteLLMProviderOptions{
			ReasoningEffort: stringPtr("low"),
		},
	}
	defaults := &codersdk.ChatModelProviderOptions{
		LiteLLM: &codersdk.ChatModelLiteLLMProviderOptions{
			User:            stringPtr("default-user"),
			ReasoningEffort: stringPtr("high"),
		},
	}

	chatprovider.MergeMissingProviderOptions(&options, defaults)

	require.NotNil(t, options)
	require.NotNil(t, options.LiteLLM)
	// ReasoningEffort was already set, so it should keep the original.
	require.Equal(t, "low", *options.LiteLLM.ReasoningEffort)
	// User was nil, so it should be filled from defaults.
	require.Equal(t, "default-user", *options.LiteLLM.User)
}

func TestMergeMissingProviderOptions_LiteLLMNilDst(t *testing.T) {
	t.Parallel()

	options := &codersdk.ChatModelProviderOptions{}
	defaults := &codersdk.ChatModelProviderOptions{
		LiteLLM: &codersdk.ChatModelLiteLLMProviderOptions{
			User:            stringPtr("default-user"),
			ReasoningEffort: stringPtr("medium"),
		},
	}

	chatprovider.MergeMissingProviderOptions(&options, defaults)

	require.NotNil(t, options.LiteLLM)
	require.Equal(t, "default-user", *options.LiteLLM.User)
	require.Equal(t, "medium", *options.LiteLLM.ReasoningEffort)
}

func TestProviderDisplayName_LiteLLM(t *testing.T) {
	t.Parallel()

	require.Equal(t, "LiteLLM", chatprovider.ProviderDisplayName("litellm"))
	require.Equal(t, "LiteLLM", chatprovider.ProviderDisplayName("LITELLM"))
	require.Equal(t, "LiteLLM", chatprovider.ProviderDisplayName(" LiteLLM "))
}

func TestMergeProviderAPIKeys_LiteLLM(t *testing.T) {
	t.Parallel()

	fallback := chatprovider.ProviderAPIKeys{
		ByProvider:        map[string]string{"litellm": "fallback-key"},
		BaseURLByProvider: map[string]string{"litellm": "http://localhost:4000"},
	}
	providers := []chatprovider.ConfiguredProvider{
		{
			Provider: "litellm",
			APIKey:   "configured-key",
			BaseURL:  "http://litellm.internal:4000",
		},
	}

	merged := chatprovider.MergeProviderAPIKeys(fallback, providers)

	// Configured provider key overrides fallback.
	require.Equal(t, "configured-key", merged.APIKey("litellm"))
	require.Equal(t, "http://litellm.internal:4000", merged.BaseURL("litellm"))
}

func TestMergeProviderAPIKeys_LiteLLMFallback(t *testing.T) {
	t.Parallel()

	fallback := chatprovider.ProviderAPIKeys{
		ByProvider:        map[string]string{"litellm": "fallback-key"},
		BaseURLByProvider: map[string]string{"litellm": "http://localhost:4000"},
	}

	// No configured providers — fallback keys are preserved.
	merged := chatprovider.MergeProviderAPIKeys(fallback, nil)

	require.Equal(t, "fallback-key", merged.APIKey("litellm"))
	require.Equal(t, "http://localhost:4000", merged.BaseURL("litellm"))
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}
