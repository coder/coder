package chatd_test

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
