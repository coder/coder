package chatprovider_test

import (
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestResolveReasoningEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested *string
		config    *codersdk.ChatModelReasoningEffortConfig
		want      *string
	}{
		{name: "NilConfigIgnoresRequested", requested: new(codersdk.ChatModelReasoningEffortHigh)},
		{name: "DefaultUsedWhenNoRequested", config: effortConfig("medium", "high"), want: new(codersdk.ChatModelReasoningEffortMedium)},
		{name: "RequestedWinsOverDefault", requested: new(codersdk.ChatModelReasoningEffortHigh), config: effortConfig("medium", "high"), want: new(codersdk.ChatModelReasoningEffortHigh)},
		{name: "RequestedWinsWithoutMax", requested: new(codersdk.ChatModelReasoningEffortHigh), config: effortConfig("medium", ""), want: new(codersdk.ChatModelReasoningEffortHigh)},
		{name: "RequestedClampedToMax", requested: new(codersdk.ChatModelReasoningEffortXHigh), config: effortConfig("low", "medium"), want: new(codersdk.ChatModelReasoningEffortMedium)},
		{name: "DefaultClampedToMax", config: effortConfig("xhigh", "medium"), want: new(codersdk.ChatModelReasoningEffortMedium)},
		{name: "InvalidRequestedFallsBackToDefault", requested: ptr.Ref(" HIGH "), config: effortConfig("low", "high"), want: new(codersdk.ChatModelReasoningEffortLow)},
		{name: "InvalidMaxReturnsNil", requested: new(codersdk.ChatModelReasoningEffortMedium), config: effortConfig("low", " HIGH ")},
		{name: "EmptyConfigReturnsNil", config: &codersdk.ChatModelReasoningEffortConfig{}},
		{name: "MaxSupported", requested: new(codersdk.ChatModelReasoningEffortMax), config: effortConfig("medium", "max"), want: new(codersdk.ChatModelReasoningEffortMax)},
		{name: "NoneSupported", requested: new(codersdk.ChatModelReasoningEffortNone), config: effortConfig("medium", "xhigh"), want: new(codersdk.ChatModelReasoningEffortNone)},
		{name: "MaxOnlyConfigClampsRequested", requested: new(codersdk.ChatModelReasoningEffortXHigh), config: effortConfig("", "medium"), want: new(codersdk.ChatModelReasoningEffortMedium)},
		{name: "MaxOnlyConfigWithoutRequestedReturnsNil", config: effortConfig("", "medium")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatprovider.ResolveReasoningEffort(tt.requested, tt.config)
			if tt.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tt.want, *got)
		})
	}
}

func TestSelectableReasoningEfforts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config *codersdk.ChatModelReasoningEffortConfig
		want   []string
	}{
		{name: "NilConfig"},
		{name: "NoMax", config: effortConfig("medium", "")},
		{name: "UnknownMax", config: effortConfig("medium", " HIGH ")},
		{name: "ThroughMedium", config: effortConfig("low", "medium"), want: []string{"none", "minimal", "low", "medium"}},
		{name: "ThroughMax", config: effortConfig("medium", "max"), want: []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chatprovider.SelectableReasoningEfforts(tt.config))
		})
	}
}

func TestApplyReasoningEffort(t *testing.T) {
	t.Parallel()

	t.Run("CreatesOpenAIResponsesEntry", func(t *testing.T) {
		t.Parallel()

		got := chatprovider.ApplyReasoningEffort(&chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "gpt-5"}, nil, new(codersdk.ChatModelReasoningEffortHigh))
		providerOptions, ok := got[fantasyopenai.Name].(*fantasyopenai.ResponsesProviderOptions)
		require.True(t, ok, "%T", got[fantasyopenai.Name])
		require.NotNil(t, providerOptions.ReasoningEffort)
		require.Equal(t, fantasyopenai.ReasoningEffortHigh, *providerOptions.ReasoningEffort)
	})

	t.Run("PreservesOpenAIResponsesEntry", func(t *testing.T) {
		t.Parallel()

		options := fantasy.ProviderOptions{
			fantasyopenai.Name: &fantasyopenai.ResponsesProviderOptions{
				Instructions: ptr.Ref("answer briefly"),
				Store:        ptr.Ref(true),
			},
		}
		got := chatprovider.ApplyReasoningEffort(&chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "gpt-5"}, options, new(codersdk.ChatModelReasoningEffortHigh))
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
				User:              ptr.Ref("user"),
				ParallelToolCalls: ptr.Ref(true),
			},
		}
		got := chatprovider.ApplyReasoningEffort(&chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "gpt-4"}, options, new(codersdk.ChatModelReasoningEffortHigh))
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
			options:  fantasy.ProviderOptions{fantasyanthropic.Name: &fantasyanthropic.ProviderOptions{SendReasoning: ptr.Ref(true)}},
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
			options:  fantasy.ProviderOptions{fantasyopenaicompat.Name: &fantasyopenaicompat.ProviderOptions{User: ptr.Ref("user")}},
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
			options:  fantasy.ProviderOptions{fantasyvercel.Name: &fantasyvercel.ProviderOptions{Reasoning: &fantasyvercel.ReasoningOptions{Enabled: ptr.Ref(true), MaxTokens: ptr.Ref(int64(1024))}}},
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
			options:  fantasy.ProviderOptions{fantasyopenrouter.Name: &fantasyopenrouter.ProviderOptions{Reasoning: &fantasyopenrouter.ReasoningOptions{Enabled: ptr.Ref(true), MaxTokens: ptr.Ref(int64(1024))}}},
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
			got := chatprovider.ApplyReasoningEffort(&chattest.FakeModel{ProviderName: tt.provider}, tt.options, new(codersdk.ChatModelReasoningEffortHigh))
			tt.assert(t, got)
		})
	}
}

func effortConfig(defaultEffort, maxEffort string) *codersdk.ChatModelReasoningEffortConfig {
	cfg := &codersdk.ChatModelReasoningEffortConfig{}
	if defaultEffort != "" {
		cfg.Default = ptr.Ref(defaultEffort)
	}
	if maxEffort != "" {
		cfg.Max = ptr.Ref(maxEffort)
	}
	return cfg
}
