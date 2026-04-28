package chatsanitize

import (
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/require"
)

func textMessageForTest(role fantasy.MessageRole, text string) fantasy.Message {
	return fantasy.Message{
		Role: role,
		Content: []fantasy.MessagePart{
			fantasy.TextPart{Text: text},
		},
	}
}

func TestProviderExecutedToolMessageIndexes(t *testing.T) {
	t.Parallel()

	messages := []fantasy.Message{
		textMessageForTest(fantasy.MessageRoleUser, "plain"),
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.ToolResultPart{
					ToolCallID:       "ws-result-only",
					ProviderExecuted: true,
				},
			},
		},
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.ToolCallPart{
					ToolCallID:       "ws-call",
					ToolName:         "web_search",
					Input:            `{"query":"coder"}`,
					ProviderExecuted: true,
				},
			},
		},
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				fantasy.ToolCallPart{
					ToolCallID: "local-call",
					ToolName:   "read_file",
					Input:      `{"path":"main.go"}`,
				},
			},
		},
	}

	require.Equal(t, map[int]struct{}{1: {}, 2: {}}, providerExecutedToolMessageIndexes(messages))
}

func TestAnthropicProviderToolFallbackStripHelpers(t *testing.T) {
	t.Parallel()

	providerCall := fantasy.ToolCallPart{
		ToolCallID:       "ws-strip",
		ToolName:         "web_search",
		Input:            `{"query":"coder"}`,
		ProviderExecuted: true,
	}
	providerResult := fantasy.ToolResultPart{
		ToolCallID:       "ws-strip",
		Output:           fantasy.ToolResultOutputContentText{Text: "ok"},
		ProviderExecuted: true,
	}
	messages := []fantasy.Message{
		textMessageForTest(fantasy.MessageRoleAssistant, "first"),
		{
			Role: fantasy.MessageRoleAssistant,
			Content: []fantasy.MessagePart{
				providerCall,
				providerResult,
			},
		},
		textMessageForTest(fantasy.MessageRoleAssistant, "second"),
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "keep"},
				fantasy.ToolResultPart{
					ToolCallID:       "ws-user",
					ProviderExecuted: true,
				},
			},
		},
	}

	stripped, stats := stripAnthropicProviderToolHistoryFromMessages(
		messages,
		map[int]struct{}{1: {}, 3: {}},
	)
	require.Equal(t, 1, stats.RemovedToolCalls)
	require.Equal(t, 2, stats.RemovedToolResults)
	require.Zero(t, stats.DroppedMessages)

	sanitized, sanitizeStats := SanitizeAnthropicProviderToolHistory(
		fantasyanthropic.Name,
		stripped,
	)
	require.Zero(t, sanitizeStats.RemovedToolCalls)
	require.Zero(t, sanitizeStats.RemovedToolResults)
	require.Empty(t, ValidateAnthropicProviderToolHistory(sanitized))
	require.Len(t, sanitized, 2)
	require.Equal(t, fantasy.MessageRoleAssistant, sanitized[0].Role)
	require.Len(t, sanitized[0].Content, 3)
	firstText, ok := fantasy.AsMessagePart[fantasy.TextPart](sanitized[0].Content[0])
	require.True(t, ok)
	require.Equal(t, "first", firstText.Text)
	stripText, ok := fantasy.AsMessagePart[fantasy.TextPart](sanitized[0].Content[1])
	require.True(t, ok)
	require.Equal(t, "ok", stripText.Text)
	secondText, ok := fantasy.AsMessagePart[fantasy.TextPart](sanitized[0].Content[2])
	require.True(t, ok)
	require.Equal(t, "second", secondText.Text)
	require.Equal(t, fantasy.MessageRoleUser, sanitized[1].Role)
	require.Len(t, sanitized[1].Content, 1)
	keepText, ok := fantasy.AsMessagePart[fantasy.TextPart](sanitized[1].Content[0])
	require.True(t, ok)
	require.Equal(t, "keep", keepText.Text)

	violations := make([]AnthropicProviderToolHistoryViolation, 33)
	for i := range violations {
		violations[i] = AnthropicProviderToolHistoryViolation{
			MessageIndex: i,
			PartIndex:    i + 1,
			ID:           "ws-detail",
			Reason:       "test_reason",
		}
	}
	details, truncated := anthropicProviderToolViolationLogDetails(violations)
	require.True(t, truncated)
	require.Len(t, details, maxAnthropicProviderToolViolationLogDetails)
	require.Len(t, details[0], 4)
	require.Equal(t, 0, details[0]["message_index"])
	require.Equal(t, 1, details[0]["part_index"])
	require.Equal(t, "ws-detail", details[0]["id"])
	require.Equal(t, "test_reason", details[0]["reason"])
}
