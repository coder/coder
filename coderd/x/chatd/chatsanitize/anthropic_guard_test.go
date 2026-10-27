package chatsanitize_test

import (
	"context"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
	"github.com/coder/coder/v2/testutil"
)

func TestAnthropicProviderToolPreRequestGuard(t *testing.T) {
	t.Parallel()

	providerPair := func(id string) []fantasy.MessagePart {
		return []fantasy.MessagePart{
			fantasy.ToolCallPart{
				ToolCallID:       id,
				ToolName:         "web_search",
				Input:            `{"query":"coder"}`,
				ProviderExecuted: true,
			},
			fantasy.ToolResultPart{
				ToolCallID:       id,
				Output:           fantasy.ToolResultOutputContentText{Text: "ok"},
				ProviderExecuted: true,
				ProviderOptions:  fantasy.ProviderOptions(validWebSearchProviderMetadataForTest()),
			},
		}
	}

	t.Run("orphan provider result is textified", func(t *testing.T) {
		t.Parallel()

		guarded, err := chatsanitize.ApplyAnthropicProviderToolGuard(
			context.Background(),
			slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			fantasyanthropic.Name,
			"claude-test",
			[]fantasy.Message{
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						fantasy.TextPart{Text: "keep"},
						fantasy.ToolResultPart{
							ToolCallID:       "ws-orphan",
							Output:           fantasy.ToolResultOutputContentText{Text: "search result"},
							ProviderExecuted: true,
						},
					},
				},
			},
		)
		require.NoError(t, err)

		requireNoProviderExecutedToolResultPrompt(t, guarded)
		requireAnthropicProviderToolPromptSafe(t, guarded)
		require.Len(t, guarded, 1)
		require.Len(t, guarded[0].Content, 2)
		textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](guarded[0].Content[0])
		require.True(t, ok)
		require.Equal(t, "keep", textPart.Text)
		textPart, ok = fantasy.AsMessagePart[fantasy.TextPart](guarded[0].Content[1])
		require.True(t, ok)
		require.Equal(t, "search result", textPart.Text)
	})

	t.Run("valid provider history is unchanged", func(t *testing.T) {
		t.Parallel()

		content := []fantasy.MessagePart{fantasy.TextPart{Text: "keep"}}
		content = append(content, providerPair("ws-one")...)
		content = append(content, providerPair("ws-two")...)
		guarded, err := chatsanitize.ApplyAnthropicProviderToolGuard(
			context.Background(),
			slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			fantasyanthropic.Name,
			"claude-test",
			[]fantasy.Message{{Role: fantasy.MessageRoleAssistant, Content: content}},
		)
		require.NoError(t, err)

		requireAnthropicProviderToolPromptSafe(t, guarded)
		require.Len(t, guarded, 1)
		require.Len(t, guarded[0].Content, len(content))
		requireProviderExecutedToolCallPrompt(t, guarded, "ws-one")
		requireProviderExecutedToolResultPrompt(t, guarded, "ws-one")
		requireProviderExecutedToolCallPrompt(t, guarded, "ws-two")
		requireProviderExecutedToolResultPrompt(t, guarded, "ws-two")
	})

	t.Run("non Anthropic providers are unchanged", func(t *testing.T) {
		t.Parallel()

		prompt := []fantasy.Message{
			{
				Role:    fantasy.MessageRoleAssistant,
				Content: providerPair("ws-other-provider"),
			},
		}
		guarded, err := chatsanitize.ApplyAnthropicProviderToolGuard(
			context.Background(),
			slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
			"fake",
			"fake-model",
			prompt,
		)
		require.NoError(t, err)
		require.Equal(t, prompt, guarded)
	})

	t.Run("logs removals", func(t *testing.T) {
		t.Parallel()

		logSink := testutil.NewFakeSink(t)
		logger := logSink.Logger()
		logPair := providerPair("ws-log")
		guarded, err := chatsanitize.ApplyAnthropicProviderToolGuard(
			context.Background(),
			logger,
			fantasyanthropic.Name,
			"claude-test",
			[]fantasy.Message{
				{
					Role: fantasy.MessageRoleAssistant,
					Content: []fantasy.MessagePart{
						logPair[1],
						logPair[0],
					},
				},
			},
		)
		require.NoError(t, err)

		requireNoProviderExecutedToolCallPrompt(t, guarded)
		requireNoProviderExecutedToolResultPrompt(t, guarded)
		requireTextPrompt(t, guarded, "ok")
		entries := logSink.Entries(func(e slog.SinkEntry) bool {
			return e.Level == slog.LevelWarn &&
				e.Message == "removed provider-executed tool history"
		})
		require.Len(t, entries, 1)
		require.Equal(t, "pre_request_guard", requireLogField(t, entries[0], "phase"))
		require.Equal(t, 1, requireLogField(t, entries[0], "removed_tool_calls"))
		require.Equal(t, 1, requireLogField(t, entries[0], "removed_tool_results"))
	})
}

func validWebSearchProviderMetadataForTest() fantasy.ProviderMetadata {
	return fantasy.ProviderMetadata{
		fantasyanthropic.Name: &fantasyanthropic.WebSearchResultMetadata{
			Results: []fantasyanthropic.WebSearchResultItem{
				{
					URL:              "https://example.com",
					Title:            "Example",
					EncryptedContent: "encrypted",
				},
			},
		},
	}
}

func safeToolCallPart(part fantasy.MessagePart) (fantasy.ToolCallPart, bool) {
	var zero fantasy.ToolCallPart
	if part == nil {
		return zero, false
	}
	if value, ok := part.(*fantasy.ToolCallPart); ok && value == nil {
		return zero, false
	}
	type toolCallPart = fantasy.ToolCallPart
	return fantasy.AsMessagePart[toolCallPart](part)
}

func safeToolResultPart(part fantasy.MessagePart) (fantasy.ToolResultPart, bool) {
	var zero fantasy.ToolResultPart
	if part == nil {
		return zero, false
	}
	if value, ok := part.(*fantasy.ToolResultPart); ok && value == nil {
		return zero, false
	}
	type toolResultPart = fantasy.ToolResultPart
	return fantasy.AsMessagePart[toolResultPart](part)
}

func requireProviderExecutedToolCallPrompt(
	t *testing.T,
	prompt []fantasy.Message,
	id string,
) fantasy.ToolCallPart {
	t.Helper()
	for _, message := range prompt {
		for _, part := range message.Content {
			toolCall, ok := safeToolCallPart(part)
			if ok && toolCall.ProviderExecuted && toolCall.ToolCallID == id {
				return toolCall
			}
		}
	}
	t.Fatalf("missing provider-executed prompt tool call %q", id)
	return fantasy.ToolCallPart{}
}

func requireProviderExecutedToolResultPrompt(
	t *testing.T,
	prompt []fantasy.Message,
	id string,
) fantasy.ToolResultPart {
	t.Helper()
	for _, message := range prompt {
		for _, part := range message.Content {
			toolResult, ok := safeToolResultPart(part)
			if ok && toolResult.ProviderExecuted && toolResult.ToolCallID == id {
				return toolResult
			}
		}
	}
	t.Fatalf("missing provider-executed prompt tool result %q", id)
	return fantasy.ToolResultPart{}
}

func requireNoProviderExecutedToolCallPrompt(t *testing.T, prompt []fantasy.Message) {
	t.Helper()
	for i, message := range prompt {
		for j, part := range message.Content {
			toolCall, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part)
			if ok && toolCall.ProviderExecuted {
				t.Fatalf("prompt[%d].content[%d]: unexpected provider-executed call", i, j)
			}
		}
	}
}

func requireNoProviderExecutedToolResultPrompt(t *testing.T, prompt []fantasy.Message) {
	t.Helper()
	for i, message := range prompt {
		for j, part := range message.Content {
			toolResult, ok := safeToolResultPart(part)
			if ok && toolResult.ProviderExecuted {
				t.Fatalf("prompt[%d].content[%d]: unexpected provider-executed result", i, j)
			}
		}
	}
}

func requireTextPrompt(t *testing.T, prompt []fantasy.Message, text string) fantasy.TextPart {
	t.Helper()
	for _, message := range prompt {
		for _, part := range message.Content {
			textPart, ok := fantasy.AsMessagePart[fantasy.TextPart](part)
			if ok && textPart.Text == text {
				return textPart
			}
		}
	}
	t.Fatalf("missing prompt text %q", text)
	return fantasy.TextPart{}
}

func requireAnthropicProviderToolPromptSafe(t *testing.T, prompt []fantasy.Message) {
	t.Helper()
	require.Empty(t, chatsanitize.ValidateAnthropicProviderToolHistory(prompt))
}

func requireLogField(t *testing.T, entry slog.SinkEntry, name string) any {
	t.Helper()
	for _, field := range entry.Fields {
		if field.Name == name {
			return field.Value
		}
	}
	t.Fatalf("missing log field %q", name)
	return nil
}
