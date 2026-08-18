//nolint:testpackage // These tests cover the unexported applyReasoningEffort.
package chatprovider

import (
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasygoogle "charm.land/fantasy/providers/google"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/ptr"
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
				Instructions: ptr.Ref("answer briefly"),
				Store:        ptr.Ref(true),
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
				User:              ptr.Ref("user"),
				ParallelToolCalls: ptr.Ref(true),
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
		name      string
		provider  string
		modelName string
		options   fantasy.ProviderOptions
		assert    func(*testing.T, fantasy.ProviderOptions)
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
			name:      "CreatesGoogleEntry",
			provider:  fantasygoogle.Name,
			modelName: "gemini-3.7-flash",
			assert: func(t *testing.T, got fantasy.ProviderOptions) {
				providerOptions, ok := got[fantasygoogle.Name].(*fantasygoogle.ProviderOptions)
				require.True(t, ok, "%T", got[fantasygoogle.Name])
				require.NotNil(t, providerOptions.ThinkingConfig)
				require.NotNil(t, providerOptions.ThinkingConfig.ThinkingLevel)
				require.Equal(t, fantasygoogle.ThinkingLevelHigh, *providerOptions.ThinkingConfig.ThinkingLevel)
			},
		},
		{
			name:      "PreservesGoogleEntry",
			provider:  fantasygoogle.Name,
			modelName: "gemini-3.7-flash",
			options:   fantasy.ProviderOptions{fantasygoogle.Name: &fantasygoogle.ProviderOptions{ThinkingConfig: &fantasygoogle.ThinkingConfig{IncludeThoughts: ptr.Ref(true)}}},
			assert: func(t *testing.T, got fantasy.ProviderOptions) {
				providerOptions := got[fantasygoogle.Name].(*fantasygoogle.ProviderOptions)
				require.True(t, *providerOptions.ThinkingConfig.IncludeThoughts)
				require.NotNil(t, providerOptions.ThinkingConfig.ThinkingLevel)
				require.Equal(t, fantasygoogle.ThinkingLevelHigh, *providerOptions.ThinkingConfig.ThinkingLevel)
			},
		},
		{
			name:      "GoogleExplicitBudgetWins",
			provider:  fantasygoogle.Name,
			modelName: "gemini-3.7-flash",
			options:   fantasy.ProviderOptions{fantasygoogle.Name: &fantasygoogle.ProviderOptions{ThinkingConfig: &fantasygoogle.ThinkingConfig{ThinkingBudget: ptr.Ref(int64(1024))}}},
			assert: func(t *testing.T, got fantasy.ProviderOptions) {
				providerOptions := got[fantasygoogle.Name].(*fantasygoogle.ProviderOptions)
				require.Equal(t, int64(1024), *providerOptions.ThinkingConfig.ThinkingBudget)
				require.Nil(t, providerOptions.ThinkingConfig.ThinkingLevel)
			},
		},
		{
			name:      "GoogleGemini25GetsNoLevel",
			provider:  fantasygoogle.Name,
			modelName: "gemini-2.5-flash",
			assert: func(t *testing.T, got fantasy.ProviderOptions) {
				require.Nil(t, got[fantasygoogle.Name])
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
			got := applyReasoningEffort(NewModel(&chattest.FakeModel{ProviderName: tt.provider, ModelName: tt.modelName}, nil), tt.options, new(codersdk.ChatModelReasoningEffortHigh))
			tt.assert(t, got)
		})
	}
}

func TestGoogleSupportedThinkingLevels(t *testing.T) {
	t.Parallel()

	minimal := fantasygoogle.ThinkingLevelMinimal
	low := fantasygoogle.ThinkingLevelLow
	medium := fantasygoogle.ThinkingLevelMedium
	high := fantasygoogle.ThinkingLevelHigh

	tests := []struct {
		modelID string
		want    []fantasygoogle.ThinkingLevel
	}{
		{modelID: "gemini-3.7-flash", want: []fantasygoogle.ThinkingLevel{minimal, low, medium, high}},
		{modelID: "gemini-3-flash-preview", want: []fantasygoogle.ThinkingLevel{minimal, low, medium, high}},
		{modelID: "models/gemini-3.7-flash", want: []fantasygoogle.ThinkingLevel{minimal, low, medium, high}},
		{modelID: " Gemini-4-Flash ", want: []fantasygoogle.ThinkingLevel{minimal, low, medium, high}},
		{modelID: "gemini-flash-latest", want: []fantasygoogle.ThinkingLevel{minimal, low, medium, high}},
		{modelID: "gemini-flash-lite-latest", want: []fantasygoogle.ThinkingLevel{minimal, low, medium, high}},
		{modelID: "gemini-3-pro-preview", want: []fantasygoogle.ThinkingLevel{low, high}},
		{modelID: "gemini-3.1-pro-preview", want: []fantasygoogle.ThinkingLevel{low, medium, high}},
		{modelID: "gemini-10.5-pro", want: []fantasygoogle.ThinkingLevel{low, medium, high}},
		{modelID: "gemini-pro-latest", want: []fantasygoogle.ThinkingLevel{low, medium, high}},
		{modelID: "gemini-3-pro-image-preview", want: []fantasygoogle.ThinkingLevel{high}},
		{modelID: "gemini-3.1-flash-image", want: []fantasygoogle.ThinkingLevel{minimal, high}},
		{modelID: "gemini-3-ultra", want: []fantasygoogle.ThinkingLevel{low, high}},
		{modelID: "gemini-2.5-flash", want: nil},
		{modelID: "gemini-2.0-flash", want: nil},
		{modelID: "gemini-1.5-flash-latest", want: nil},
		{modelID: "gemini-exp-1206", want: nil},
		{modelID: "gemma-3-27b-it", want: nil},
		{modelID: "learnlm-2.0-flash", want: nil},
		{modelID: "", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, googleSupportedThinkingLevels(tt.modelID))
		})
	}
}

func TestClampGoogleThinkingLevel(t *testing.T) {
	t.Parallel()

	gemini3Pro := googleSupportedThinkingLevels("gemini-3-pro-preview")
	proImage := googleSupportedThinkingLevels("gemini-3-pro-image-preview")
	flash := googleSupportedThinkingLevels("gemini-3.7-flash")

	tests := []struct {
		name      string
		desired   fantasygoogle.ThinkingLevel
		supported []fantasygoogle.ThinkingLevel
		want      fantasygoogle.ThinkingLevel
	}{
		{name: "MinimalRoundsUpToLowOnPro", desired: fantasygoogle.ThinkingLevelMinimal, supported: gemini3Pro, want: fantasygoogle.ThinkingLevelLow},
		{name: "MediumRoundsUpToHighOnPro", desired: fantasygoogle.ThinkingLevelMedium, supported: gemini3Pro, want: fantasygoogle.ThinkingLevelHigh},
		{name: "HighExactOnPro", desired: fantasygoogle.ThinkingLevelHigh, supported: gemini3Pro, want: fantasygoogle.ThinkingLevelHigh},
		{name: "LowRoundsUpToHighOnProImage", desired: fantasygoogle.ThinkingLevelLow, supported: proImage, want: fantasygoogle.ThinkingLevelHigh},
		{name: "MediumExactOnFlash", desired: fantasygoogle.ThinkingLevelMedium, supported: flash, want: fantasygoogle.ThinkingLevelMedium},
		{name: "MinimalExactOnFlash", desired: fantasygoogle.ThinkingLevelMinimal, supported: flash, want: fantasygoogle.ThinkingLevelMinimal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, clampGoogleThinkingLevel(tt.desired, tt.supported))
		})
	}
}

func TestGoogleThinkingLevel(t *testing.T) {
	t.Parallel()

	want := map[string]fantasygoogle.ThinkingLevel{
		codersdk.ChatModelReasoningEffortNone:    fantasygoogle.ThinkingLevelMinimal,
		codersdk.ChatModelReasoningEffortMinimal: fantasygoogle.ThinkingLevelMinimal,
		codersdk.ChatModelReasoningEffortLow:     fantasygoogle.ThinkingLevelLow,
		codersdk.ChatModelReasoningEffortMedium:  fantasygoogle.ThinkingLevelMedium,
		codersdk.ChatModelReasoningEffortHigh:    fantasygoogle.ThinkingLevelHigh,
		codersdk.ChatModelReasoningEffortXHigh:   fantasygoogle.ThinkingLevelHigh,
		codersdk.ChatModelReasoningEffortMax:     fantasygoogle.ThinkingLevelHigh,
	}
	for _, effort := range codersdk.ChatModelReasoningEffortValues() {
		require.Contains(t, want, effort, "effort %q missing an expected Google thinking level", effort)
		require.Equal(t, want[effort], googleThinkingLevel(effort), "effort %q", effort)
	}
}
