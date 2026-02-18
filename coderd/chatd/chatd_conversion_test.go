package chatd

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

func TestChatMessagesToPrompt(t *testing.T) {
	t.Parallel()

	systemContent, err := json.Marshal("system")
	require.NoError(t, err)

	userContent, err := json.Marshal(contentFromText("hello"))
	require.NoError(t, err)

	assistantBlocks := append(contentFromText("working"), fantasy.ToolCallContent{
		ToolCallID: "tool-1",
		ToolName:   toolReadFile,
		Input:      `{"path":"hello.txt"}`,
	})
	assistantContent, err := json.Marshal(assistantBlocks)
	require.NoError(t, err)

	toolResults, err := json.Marshal([]ToolResultBlock{{
		ToolCallID: "tool-1",
		ToolName:   toolReadFile,
		Result:     map[string]any{"content": "hello"},
	}})
	require.NoError(t, err)

	messages := []database.ChatMessage{
		{
			Role:    string(fantasy.MessageRoleSystem),
			Content: pqtype.NullRawMessage{RawMessage: systemContent, Valid: true},
		},
		{
			Role:    string(fantasy.MessageRoleUser),
			Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
		},
		{
			Role:    string(fantasy.MessageRoleAssistant),
			Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
		},
		{
			Role:    string(fantasy.MessageRoleTool),
			Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
		},
	}

	prompt, err := chatMessagesToPrompt(messages)
	require.NoError(t, err)
	require.Len(t, prompt, 4)
	require.Equal(t, fantasy.MessageRoleAssistant, prompt[2].Role)
	require.Len(t, extractToolCallsFromMessageParts(prompt[2].Content), 1)
}

func TestChatMessagesToPrompt_InjectsMissingToolResults(t *testing.T) {
	t.Parallel()

	t.Run("InterruptedAfterToolCall", func(t *testing.T) {
		t.Parallel()

		// Simulate an interrupted chat: assistant made tool calls but
		// the processing was interrupted before tool results were saved.
		userContent, err := json.Marshal(contentFromText("hello"))
		require.NoError(t, err)

		assistantBlocks := append(contentFromText("let me check"),
			fantasy.ToolCallContent{
				ToolCallID: "call-1",
				ToolName:   toolReadFile,
				Input:      `{"path":"main.go"}`,
			},
			fantasy.ToolCallContent{
				ToolCallID: "call-2",
				ToolName:   toolExecute,
				Input:      `{"command":"ls"}`,
			},
		)
		assistantContent, err := json.Marshal(assistantBlocks)
		require.NoError(t, err)

		messages := []database.ChatMessage{
			{
				Role:    string(fantasy.MessageRoleUser),
				Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
			},
			{
				Role:    string(fantasy.MessageRoleAssistant),
				Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
			},
		}

		prompt, err := chatMessagesToPrompt(messages)
		require.NoError(t, err)

		// Should have injected a tool message after the assistant.
		require.Len(t, prompt, 3, "expected injected tool result message")

		toolMsg := prompt[2]
		require.Equal(t, fantasy.MessageRoleTool, toolMsg.Role)
		toolResults := messageToolResultParts(toolMsg)
		require.Len(t, toolResults, 2, "should have results for both tool calls")

		for _, result := range toolResults {
			_, ok := result.Output.(fantasy.ToolResultOutputContentError)
			require.True(t, ok, "injected result should be an error")
		}
		require.Equal(t, "call-1", toolResults[0].ToolCallID)
		require.Equal(t, "call-2", toolResults[1].ToolCallID)
	})

	t.Run("PartialToolResults", func(t *testing.T) {
		t.Parallel()

		// Assistant made two tool calls but only one result was saved
		// before interruption.
		userContent, err := json.Marshal(contentFromText("hello"))
		require.NoError(t, err)

		assistantBlocks := append(contentFromText("working"),
			fantasy.ToolCallContent{
				ToolCallID: "call-1",
				ToolName:   toolReadFile,
				Input:      `{"path":"a.go"}`,
			},
			fantasy.ToolCallContent{
				ToolCallID: "call-2",
				ToolName:   toolReadFile,
				Input:      `{"path":"b.go"}`,
			},
		)
		assistantContent, err := json.Marshal(assistantBlocks)
		require.NoError(t, err)

		toolResults, err := json.Marshal([]ToolResultBlock{{
			ToolCallID: "call-1",
			ToolName:   toolReadFile,
			Result:     map[string]any{"content": "file a"},
		}})
		require.NoError(t, err)

		messages := []database.ChatMessage{
			{
				Role:    string(fantasy.MessageRoleUser),
				Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
			},
			{
				Role:    string(fantasy.MessageRoleAssistant),
				Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
			},
			{
				Role:    string(fantasy.MessageRoleTool),
				Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
			},
		}

		prompt, err := chatMessagesToPrompt(messages)
		require.NoError(t, err)

		// Original 3 messages + 1 injected tool message for call-2.
		require.Len(t, prompt, 4)

		injectedMsg := prompt[3]
		require.Equal(t, fantasy.MessageRoleTool, injectedMsg.Role)
		injectedParts := messageToolResultParts(injectedMsg)
		require.Len(t, injectedParts, 1)
		require.Equal(t, "call-2", injectedParts[0].ToolCallID)
		_, ok := injectedParts[0].Output.(fantasy.ToolResultOutputContentError)
		require.True(t, ok)
	})

	t.Run("NoToolCalls", func(t *testing.T) {
		t.Parallel()

		// Assistant message with no tool calls should not inject anything.
		userContent, err := json.Marshal(contentFromText("hi"))
		require.NoError(t, err)

		assistantContent, err := json.Marshal(contentFromText("hello back"))
		require.NoError(t, err)

		messages := []database.ChatMessage{
			{
				Role:    string(fantasy.MessageRoleUser),
				Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
			},
			{
				Role:    string(fantasy.MessageRoleAssistant),
				Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
			},
		}

		prompt, err := chatMessagesToPrompt(messages)
		require.NoError(t, err)
		require.Len(t, prompt, 2, "no injection expected when no tool calls")
	})

	t.Run("AllToolResultsPresent", func(t *testing.T) {
		t.Parallel()

		// All tool calls already have results; nothing to inject.
		userContent, err := json.Marshal(contentFromText("hello"))
		require.NoError(t, err)

		assistantBlocks := append(contentFromText("working"), fantasy.ToolCallContent{
			ToolCallID: "call-1",
			ToolName:   toolReadFile,
			Input:      `{"path":"x.go"}`,
		})
		assistantContent, err := json.Marshal(assistantBlocks)
		require.NoError(t, err)

		toolResults, err := json.Marshal([]ToolResultBlock{{
			ToolCallID: "call-1",
			ToolName:   toolReadFile,
			Result:     map[string]any{"content": "data"},
		}})
		require.NoError(t, err)

		messages := []database.ChatMessage{
			{
				Role:    string(fantasy.MessageRoleUser),
				Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
			},
			{
				Role:    string(fantasy.MessageRoleAssistant),
				Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
			},
			{
				Role:    string(fantasy.MessageRoleTool),
				Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
			},
		}

		prompt, err := chatMessagesToPrompt(messages)
		require.NoError(t, err)
		require.Len(t, prompt, 3, "no injection when all results present")
	})
}

func contentFromText(text string) []fantasy.Content {
	return []fantasy.Content{
		fantasy.TextContent{Text: text},
	}
}

func messageToolResultParts(message fantasy.Message) []fantasy.ToolResultPart {
	results := make([]fantasy.ToolResultPart, 0, len(message.Content))
	for _, part := range message.Content {
		result, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part)
		if !ok {
			continue
		}
		results = append(results, result)
	}
	return results
}
