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
			Reasoning: &codersdk.ChatModelOpenRouterReasoningOptions{
				Enabled: boolPtr(true),
			},
			Provider: &codersdk.ChatModelOpenRouterProvider{
				Order: []string{"openai"},
			},
		},
	}
	defaults := &codersdk.ChatModelProviderOptions{
		OpenRouter: &codersdk.ChatModelOpenRouterProviderOptions{
			Reasoning: &codersdk.ChatModelOpenRouterReasoningOptions{
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

func TestMergeMissingCallConfig_FillsUnsetFields(t *testing.T) {
	t.Parallel()

	dst := codersdk.ChatModelCallConfig{
		Temperature: float64Ptr(0.2),
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
				User: stringPtr("alice"),
			},
		},
	}
	defaults := codersdk.ChatModelCallConfig{
		MaxOutputTokens: int64Ptr(512),
		Temperature:     float64Ptr(0.9),
		TopP:            float64Ptr(0.8),
		ProviderOptions: &codersdk.ChatModelProviderOptions{
			OpenAI: &codersdk.ChatModelOpenAIProviderOptions{
				User:            stringPtr("bob"),
				ReasoningEffort: stringPtr("medium"),
			},
		},
	}

	chatprovider.MergeMissingCallConfig(&dst, defaults)

	require.NotNil(t, dst.MaxOutputTokens)
	require.EqualValues(t, 512, *dst.MaxOutputTokens)
	require.NotNil(t, dst.Temperature)
	require.Equal(t, 0.2, *dst.Temperature)
	require.NotNil(t, dst.TopP)
	require.Equal(t, 0.8, *dst.TopP)
	require.NotNil(t, dst.ProviderOptions)
	require.NotNil(t, dst.ProviderOptions.OpenAI)
	require.Equal(t, "alice", *dst.ProviderOptions.OpenAI.User)
	require.Equal(t, "medium", *dst.ProviderOptions.OpenAI.ReasoningEffort)
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

func float64Ptr(value float64) *float64 {
	return &value
}
