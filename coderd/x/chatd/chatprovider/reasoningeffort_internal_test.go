//nolint:testpackage // These tests cover the unexported applyReasoningEffort.
package chatprovider

import (
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestApplyReasoningEffort(t *testing.T) {
	t.Parallel()

	t.Run("CreatesOpenAIResponsesEntry", func(t *testing.T) {
		t.Parallel()

		got := applyReasoningEffort(NewModel(&chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "gpt-5"}, nil), nil, new(codersdk.ChatModelReasoningEffortHigh))
		providerOptions, ok := got[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
		require.True(t, ok, "%T", got[fantasyopenai.Name])
		require.NotNil(t, providerOptions.ReasoningEffort)
		require.Equal(t, fantasyopenai.ReasoningEffortHigh, *providerOptions.ReasoningEffort)
	})

	t.Run("PreservesOpenAIResponsesEntry", func(t *testing.T) {
		t.Parallel()

		options := fantasy.ProviderOptions{
			fantasyopenai.Name: &fantasyopenai.ResponsesProviderOptions{
				Instructions: new("answer briefly"),
				Store:        new(true),
			},
		}
		got := applyReasoningEffort(NewModel(&chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "gpt-5"}, nil), options, new(codersdk.ChatModelReasoningEffortHigh))
		providerOptions, ok := got[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
		require.True(t, ok, "%T", got[fantasyopenai.Name])
		require.Same(t, options[fantasyopenai.Name], providerOptions)
		require.Equal(t, "answer briefly", *providerOptions.Instructions)
		require.True(t, *providerOptions.Store)
		require.Equal(t, fantasyopenai.ReasoningEffortHigh, *providerOptions.ReasoningEffort)
	})

	t.Run("PreservesOpenAILegacyEntry", func(t *testing.T) {
		t.Parallel()

		options := fantasy.ProviderOptions{
			fantasyopenai.Name: &fantasyopenai.ProviderOptions{
				User:              new("user"),
				ParallelToolCalls: new(true),
			},
		}
		got := applyReasoningEffort(NewModel(&chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "gpt-4"}, nil), options, new(codersdk.ChatModelReasoningEffortHigh))
		providerOptions, ok := got[fantasyopenai.Name].(*fantasyopenai.ProviderOptions)
		require.True(t, ok, "%T", got[fantasyopenai.Name])
		require.Same(t, options[fantasyopenai.Name], providerOptions)
		require.Equal(t, "user", *providerOptions.User)
		require.True(t, *providerOptions.ParallelToolCalls)
		require.Equal(t, fantasyopenai.ReasoningEffortHigh, *providerOptions.ReasoningEffort)
	})

	tests := []struct {
		name     string
		provider string
		options  fantasy.ProviderOptions
		assert   func(*testing.T, fantasy.ProviderOptions)
	}{
		{
			name:     "CreatesAnthropicEntry",
			provider: fantasyanthropic.Name,
			assert: func(t *testing.T, got fantasy.ProviderOptions) {
				providerOptions, ok := got[fantasyanthropic.Name].(*fantasyanthropic.ProviderOptions)
				require.True(t, ok, "%T", got[fantasyanthropic.Name])
				require.NotNil(t, providerOptions.Effort)
				require.Equal(t, fantasyanthropic.EffortHigh, *providerOptions.Effort)
			},
		},
		{
			name:     "PreservesAnthropicEntry",
			provider: fantasyanthropic.Name,
			options:  fantasy.ProviderOptions{fantasyanthropic.Name: &fantasyanthropic.ProviderOptions{SendReasoning: new(true)}},
			assert: func(t *testing.T, got fantasy.ProviderOptions) {
				providerOptions := got[fantasyanthropic.Name].(*fantasyanthropic.ProviderOptions)
				require.True(t, *providerOptions.SendReasoning)
				require.Equal(t, fantasyanthropic.EffortHigh, *providerOptions.Effort)
			},
		},
		{
			name:     "CreatesOpenAICompatEntry",
			provider: fantasyopenaicompat.Name,
			assert: func(t *testing.T, got fantasy.ProviderOptions) {
				providerOptions, ok := got[fantasyopenaicompat.Name].(*fantasyopenaicompat.ProviderOptions)
				require.True(t, ok, "%T", got[fantasyopenaicompat.Name])
				require.NotNil(t, providerOptions.ReasoningEffort)
				require.Equal(t, fantasyopenai.ReasoningEffortHigh, *providerOptions.ReasoningEffort)
			},
		},
		{
			name:     "PreservesOpenAICompatEntry",
			provider: fantasyopenaicompat.Name,
			options:  fantasy.ProviderOptions{fantasyopenaicompat.Name: &fantasyopenaicompat.ProviderOptions{User: new("user")}},
			assert: func(t *testing.T, got fantasy.ProviderOptions) {
				providerOptions := got[fantasyopenaicompat.Name].(*fantasyopenaicompat.ProviderOptions)
				require.Equal(t, "user", *providerOptions.User)
				require.Equal(t, fantasyopenai.ReasoningEffortHigh, *providerOptions.ReasoningEffort)
			},
		},
		{
			name:     "CreatesVercelEntry",
			provider: fantasyvercel.Name,
			assert: func(t *testing.T, got fantasy.ProviderOptions) {
				providerOptions, ok := got[fantasyvercel.Name].(*fantasyvercel.ProviderOptions)
				require.True(t, ok, "%T", got[fantasyvercel.Name])
				require.NotNil(t, providerOptions.Reasoning)
				require.NotNil(t, providerOptions.Reasoning.Effort)
				require.Equal(t, fantasyvercel.ReasoningEffortHigh, *providerOptions.Reasoning.Effort)
			},
		},
		{
			name:     "PreservesVercelNestedEntry",
			provider: fantasyvercel.Name,
			options:  fantasy.ProviderOptions{fantasyvercel.Name: &fantasyvercel.ProviderOptions{Reasoning: &fantasyvercel.ReasoningOptions{Enabled: new(true), MaxTokens: new(int64(1024))}}},
			assert: func(t *testing.T, got fantasy.ProviderOptions) {
				providerOptions := got[fantasyvercel.Name].(*fantasyvercel.ProviderOptions)
				require.True(t, *providerOptions.Reasoning.Enabled)
				require.Equal(t, int64(1024), *providerOptions.Reasoning.MaxTokens)
				require.Equal(t, fantasyvercel.ReasoningEffortHigh, *providerOptions.Reasoning.Effort)
			},
		},
		{
			name:     "CreatesOpenRouterEntry",
			provider: fantasyopenrouter.Name,
			assert: func(t *testing.T, got fantasy.ProviderOptions) {
				providerOptions, ok := got[fantasyopenrouter.Name].(*fantasyopenrouter.ProviderOptions)
				require.True(t, ok, "%T", got[fantasyopenrouter.Name])
				require.NotNil(t, providerOptions.Reasoning)
				require.NotNil(t, providerOptions.Reasoning.Effort)
				require.Equal(t, fantasyopenrouter.ReasoningEffortHigh, *providerOptions.Reasoning.Effort)
			},
		},
		{
			name:     "PreservesOpenRouterNestedEntry",
			provider: fantasyopenrouter.Name,
			options:  fantasy.ProviderOptions{fantasyopenrouter.Name: &fantasyopenrouter.ProviderOptions{Reasoning: &fantasyopenrouter.ReasoningOptions{Enabled: new(true), MaxTokens: new(int64(1024))}}},
			assert: func(t *testing.T, got fantasy.ProviderOptions) {
				providerOptions, ok := got[fantasyopenrouter.Name].(*fantasyopenrouter.ProviderOptions)
				require.True(t, ok, "%T", got[fantasyopenrouter.Name])
				require.True(t, *providerOptions.Reasoning.Enabled)
				require.Equal(t, int64(1024), *providerOptions.Reasoning.MaxTokens)
				require.NotNil(t, providerOptions.Reasoning.Effort)
				require.Equal(t, fantasyopenrouter.ReasoningEffortHigh, *providerOptions.Reasoning.Effort)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := applyReasoningEffort(NewModel(&chattest.FakeModel{ProviderName: tt.provider}, nil), tt.options, new(codersdk.ChatModelReasoningEffortHigh))
			tt.assert(t, got)
		})
	}
}
