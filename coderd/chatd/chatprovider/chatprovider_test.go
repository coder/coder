package chatprovider_test

import (
	"testing"

	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/util/ptr"
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
			input:    ptr.Ref(" HIGH "),
			want:     ptr.Ref(string(fantasyopenai.ReasoningEffortHigh)),
		},
		{
			name:     "AnthropicEffort",
			provider: "anthropic",
			input:    ptr.Ref("max"),
			want:     ptr.Ref(string(fantasyanthropic.EffortMax)),
		},
		{
			name:     "OpenRouterEffort",
			provider: "openrouter",
			input:    ptr.Ref("medium"),
			want:     ptr.Ref(string(fantasyopenrouter.ReasoningEffortMedium)),
		},
		{
			name:     "VercelEffort",
			provider: "vercel",
			input:    ptr.Ref("xhigh"),
			want:     ptr.Ref(string(fantasyvercel.ReasoningEffortXHigh)),
		},
		{
			name:     "InvalidEffortReturnsNil",
			provider: "openai",
			input:    ptr.Ref("unknown"),
			want:     nil,
		},
		{
			name:     "UnsupportedProviderReturnsNil",
			provider: "bedrock",
			input:    ptr.Ref("high"),
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
				Enabled: ptr.Ref(true),
			},
			Provider: &codersdk.ChatModelOpenRouterProvider{
				Order: []string{"openai"},
			},
		},
	}
	defaults := &codersdk.ChatModelProviderOptions{
		OpenRouter: &codersdk.ChatModelOpenRouterProviderOptions{
			Reasoning: &codersdk.ChatModelReasoningOptions{
				Enabled:   ptr.Ref(false),
				Exclude:   ptr.Ref(true),
				MaxTokens: ptr.Ref[int64](123),
				Effort:    ptr.Ref("high"),
			},
			IncludeUsage: ptr.Ref(true),
			Provider: &codersdk.ChatModelOpenRouterProvider{
				Order:             []string{"anthropic"},
				AllowFallbacks:    ptr.Ref(true),
				RequireParameters: ptr.Ref(false),
				DataCollection:    ptr.Ref("allow"),
				Only:              []string{"openai"},
				Ignore:            []string{"foo"},
				Quantizations:     []string{"int8"},
				Sort:              ptr.Ref("latency"),
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
