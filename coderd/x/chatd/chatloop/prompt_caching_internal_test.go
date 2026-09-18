package chatloop

import (
	"slices"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

func TestPromptCachingStrategyFor(t *testing.T) {
	t.Parallel()

	// Strategies are told apart by the marker they leave on a minimal prompt.
	const (
		strategyNone         = "none"
		strategyAnthropic    = "anthropic"
		strategyOpenAICompat = "openai-compat"
	)
	classify := func(strategy promptCachingStrategy) string {
		if strategy == nil {
			return strategyNone
		}
		prompt := []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")}
		strategy(prompt)
		if prompt[0].ProviderOptions[fantasyanthropic.Name] != nil {
			return strategyAnthropic
		}
		if prompt[0].Content[0].Options()[fantasyopenaicompat.Name] != nil {
			return strategyOpenAICompat
		}
		return strategyNone
	}

	tests := []struct {
		provider string
		model    string
		want     string
	}{
		{provider: "anthropic", model: "claude-sonnet-4-5", want: strategyAnthropic},
		{provider: "openrouter", model: "anthropic/claude-haiku-4.5", want: strategyAnthropic},
		{provider: "vercel", model: "anthropic/claude-haiku-4.5", want: strategyAnthropic},
		{provider: "openrouter", model: "openai/gpt-5-mini", want: strategyNone},
		{provider: "openai-compat", model: "anthropic/claude-haiku-4.5", want: strategyOpenAICompat},
		{provider: "openai-compat", model: "claude-opus-4-6", want: strategyOpenAICompat},
		{provider: "openai-compat", model: "openai/gpt-5-mini", want: strategyNone},
		{provider: "openai", model: "anthropic/claude-haiku-4.5", want: strategyNone},
		{provider: "google", model: "gemini-2.5-flash", want: strategyNone},
	}
	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.model, func(t *testing.T) {
			t.Parallel()
			model := &chattest.FakeModel{ProviderName: tt.provider, ModelName: tt.model}
			require.Equal(t, tt.want, classify(promptCachingStrategyFor(model, tt.model)))
		})
	}

	t.Run("NilModel", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, promptCachingStrategyFor(nil, "claude-opus-4-6"))
	})
}

func openAICompatCacheMarkerIndexes(msg fantasy.Message) []int {
	var marked []int
	for j, part := range msg.Content {
		if part.Options()[fantasyopenaicompat.Name] != nil {
			marked = append(marked, j)
		}
	}
	return marked
}

func TestAddOpenAICompatPromptCaching(t *testing.T) {
	t.Parallel()

	toolCall := fantasy.ToolCallPart{ToolCallID: "call-1", ToolName: "read_file", Input: "{}"}
	toolResult := fantasy.Message{
		Role: fantasy.MessageRoleTool,
		Content: []fantasy.MessagePart{fantasy.ToolResultPart{
			ToolCallID: "call-1",
			Output:     fantasy.ToolResultOutputContentText{Text: "file body"},
		}},
	}

	t.Run("SystemAndLastTwoTextBearingMessages", func(t *testing.T) {
		t.Parallel()
		prompt := []fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "sys-1"),
			{Role: fantasy.MessageRoleSystem, Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "sys-2a"},
				fantasy.TextPart{Text: "sys-2b"},
				fantasy.TextPart{Text: "   "},
			}},
			textMessage(fantasy.MessageRoleUser, "first question"),
			textMessage(fantasy.MessageRoleAssistant, "first answer"),
			textMessage(fantasy.MessageRoleUser, "second question"),
			{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{toolCall}},
			toolResult,
		}

		addOpenAICompatPromptCaching(prompt)

		require.Empty(t, openAICompatCacheMarkerIndexes(prompt[0]), "only the last system message is marked")
		require.Equal(t, []int{1}, openAICompatCacheMarkerIndexes(prompt[1]), "last non-blank text part of the last system message")
		require.Empty(t, openAICompatCacheMarkerIndexes(prompt[2]))
		require.Equal(t, []int{0}, openAICompatCacheMarkerIndexes(prompt[3]))
		require.Equal(t, []int{0}, openAICompatCacheMarkerIndexes(prompt[4]))
		require.Empty(t, openAICompatCacheMarkerIndexes(prompt[5]), "tool-call-only assistant message is skipped")
		require.Empty(t, openAICompatCacheMarkerIndexes(prompt[6]), "tool message is skipped")
		for _, msg := range prompt {
			require.Nil(t, msg.ProviderOptions[fantasyopenaicompat.Name], "markers are part-level only")
		}

		marker, ok := prompt[4].Content[0].Options()[fantasyopenaicompat.Name].(*fantasyopenaicompat.ContentExtraFields)
		require.True(t, ok)
		require.Equal(t, map[string]any{"cache_control": map[string]string{"type": "ephemeral"}}, marker.Fields)
	})

	// Each step re-prepares the prompt from the canonical messages, so the
	// breakpoints must follow the conversation tail.
	t.Run("MarkersMoveAsConversationGrows", func(t *testing.T) {
		t.Parallel()
		canonical := []fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "sys"),
			textMessage(fantasy.MessageRoleUser, "first question"),
			textMessage(fantasy.MessageRoleAssistant, "first answer"),
		}
		first := slices.Clone(canonical)
		addOpenAICompatPromptCaching(first)
		require.Equal(t, []int{0}, openAICompatCacheMarkerIndexes(first[1]))
		require.Equal(t, []int{0}, openAICompatCacheMarkerIndexes(first[2]))

		canonical = append(canonical,
			textMessage(fantasy.MessageRoleUser, "second question"),
			textMessage(fantasy.MessageRoleAssistant, "second answer"),
		)
		second := slices.Clone(canonical)
		addOpenAICompatPromptCaching(second)
		require.Equal(t, []int{0}, openAICompatCacheMarkerIndexes(second[0]))
		require.Empty(t, openAICompatCacheMarkerIndexes(second[1]))
		require.Empty(t, openAICompatCacheMarkerIndexes(second[2]))
		require.Equal(t, []int{0}, openAICompatCacheMarkerIndexes(second[3]))
		require.Equal(t, []int{0}, openAICompatCacheMarkerIndexes(second[4]))
	})

	t.Run("DoesNotMutateCanonicalMessages", func(t *testing.T) {
		t.Parallel()
		canonical := []fantasy.Message{
			textMessage(fantasy.MessageRoleSystem, "sys"),
			{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{
				Text: "question",
				ProviderOptions: fantasy.ProviderOptions{
					fantasyopenaicompat.Name: &fantasyopenaicompat.ContentExtraFields{Fields: map[string]any{"stale": true}},
				},
			}}},
		}
		prompt := make([]fantasy.Message, len(canonical))
		copy(prompt, canonical)

		addOpenAICompatPromptCaching(prompt)

		require.Empty(t, openAICompatCacheMarkerIndexes(canonical[0]))
		stale, ok := canonical[1].Content[0].Options()[fantasyopenaicompat.Name].(*fantasyopenaicompat.ContentExtraFields)
		require.True(t, ok)
		require.Equal(t, map[string]any{"stale": true}, stale.Fields)
	})
}
