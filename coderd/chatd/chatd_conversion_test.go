package chatd

import (
	"encoding/json"
	"testing"

	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.jetify.com/ai/api"

	"github.com/coder/coder/v2/coderd/database"
)

func TestChatMessagesToPrompt(t *testing.T) {
	t.Parallel()

	systemContent, err := json.Marshal("system")
	require.NoError(t, err)

	userContent, err := json.Marshal(api.ContentFromText("hello"))
	require.NoError(t, err)

	assistantBlocks := append(api.ContentFromText("working"), &api.ToolCallBlock{
		ToolCallID: "tool-1",
		ToolName:   toolReadFile,
		Args:       json.RawMessage(`{"path":"hello.txt"}`),
	})
	assistantContent, err := json.Marshal(assistantBlocks)
	require.NoError(t, err)

	toolResults, err := json.Marshal([]api.ToolResultBlock{{
		ToolCallID: "tool-1",
		ToolName:   toolReadFile,
		Result:     map[string]any{"content": "hello"},
	}})
	require.NoError(t, err)

	messages := []database.ChatMessage{
		{
			Role:    string(api.MessageRoleSystem),
			Content: pqtype.NullRawMessage{RawMessage: systemContent, Valid: true},
		},
		{
			Role:    string(api.MessageRoleUser),
			Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
		},
		{
			Role:    string(api.MessageRoleAssistant),
			Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
		},
		{
			Role:    string(api.MessageRoleTool),
			Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
		},
	}

	prompt, err := chatMessagesToPrompt(messages)
	require.NoError(t, err)
	require.Len(t, prompt, 4)

	assistantMsg, ok := prompt[2].(*api.AssistantMessage)
	require.True(t, ok)
	require.Len(t, extractToolCalls(assistantMsg.Content), 1)
}

func TestChatMessagesToPrompt_InjectsMissingToolResults(t *testing.T) {
	t.Parallel()

	t.Run("InterruptedAfterToolCall", func(t *testing.T) {
		t.Parallel()

		// Simulate an interrupted chat: assistant made tool calls
		// but the processing was interrupted before tool results
		// were saved.
		userContent, err := json.Marshal(api.ContentFromText("hello"))
		require.NoError(t, err)

		assistantBlocks := append(api.ContentFromText("let me check"),
			&api.ToolCallBlock{
				ToolCallID: "call-1",
				ToolName:   toolReadFile,
				Args:       json.RawMessage(`{"path":"main.go"}`),
			},
			&api.ToolCallBlock{
				ToolCallID: "call-2",
				ToolName:   toolExecute,
				Args:       json.RawMessage(`{"command":"ls"}`),
			},
		)
		assistantContent, err := json.Marshal(assistantBlocks)
		require.NoError(t, err)

		messages := []database.ChatMessage{
			{
				Role:    string(api.MessageRoleUser),
				Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
			},
			{
				Role:    string(api.MessageRoleAssistant),
				Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
			},
		}

		prompt, err := chatMessagesToPrompt(messages)
		require.NoError(t, err)

		// Should have injected a tool message after the assistant.
		require.Len(t, prompt, 3, "expected injected tool result message")

		toolMsg, ok := prompt[2].(*api.ToolMessage)
		require.True(t, ok, "third message should be a tool message")
		require.Len(t, toolMsg.Content, 2, "should have results for both tool calls")

		for _, result := range toolMsg.Content {
			require.True(t, result.IsError, "injected result should be an error")
		}
		require.Equal(t, "call-1", toolMsg.Content[0].ToolCallID)
		require.Equal(t, "call-2", toolMsg.Content[1].ToolCallID)
	})

	t.Run("PartialToolResults", func(t *testing.T) {
		t.Parallel()

		// Assistant made two tool calls but only one result was
		// saved before interruption.
		userContent, err := json.Marshal(api.ContentFromText("hello"))
		require.NoError(t, err)

		assistantBlocks := append(api.ContentFromText("working"),
			&api.ToolCallBlock{
				ToolCallID: "call-1",
				ToolName:   toolReadFile,
				Args:       json.RawMessage(`{"path":"a.go"}`),
			},
			&api.ToolCallBlock{
				ToolCallID: "call-2",
				ToolName:   toolReadFile,
				Args:       json.RawMessage(`{"path":"b.go"}`),
			},
		)
		assistantContent, err := json.Marshal(assistantBlocks)
		require.NoError(t, err)

		toolResults, err := json.Marshal([]api.ToolResultBlock{{
			ToolCallID: "call-1",
			ToolName:   toolReadFile,
			Result:     map[string]any{"content": "file a"},
		}})
		require.NoError(t, err)

		messages := []database.ChatMessage{
			{
				Role:    string(api.MessageRoleUser),
				Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
			},
			{
				Role:    string(api.MessageRoleAssistant),
				Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
			},
			{
				Role:    string(api.MessageRoleTool),
				Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
			},
		}

		prompt, err := chatMessagesToPrompt(messages)
		require.NoError(t, err)

		// Original 3 messages + 1 injected tool message for call-2.
		require.Len(t, prompt, 4)

		injectedMsg, ok := prompt[3].(*api.ToolMessage)
		require.True(t, ok, "fourth message should be injected tool message")
		require.Len(t, injectedMsg.Content, 1)
		require.Equal(t, "call-2", injectedMsg.Content[0].ToolCallID)
		require.True(t, injectedMsg.Content[0].IsError)
	})

	t.Run("NoToolCalls", func(t *testing.T) {
		t.Parallel()

		// Assistant message with no tool calls should not inject
		// anything.
		userContent, err := json.Marshal(api.ContentFromText("hi"))
		require.NoError(t, err)

		assistantContent, err := json.Marshal(api.ContentFromText("hello back"))
		require.NoError(t, err)

		messages := []database.ChatMessage{
			{
				Role:    string(api.MessageRoleUser),
				Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
			},
			{
				Role:    string(api.MessageRoleAssistant),
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
		userContent, err := json.Marshal(api.ContentFromText("hello"))
		require.NoError(t, err)

		assistantBlocks := append(api.ContentFromText("working"), &api.ToolCallBlock{
			ToolCallID: "call-1",
			ToolName:   toolReadFile,
			Args:       json.RawMessage(`{"path":"x.go"}`),
		})
		assistantContent, err := json.Marshal(assistantBlocks)
		require.NoError(t, err)

		toolResults, err := json.Marshal([]api.ToolResultBlock{{
			ToolCallID: "call-1",
			ToolName:   toolReadFile,
			Result:     map[string]any{"content": "data"},
		}})
		require.NoError(t, err)

		messages := []database.ChatMessage{
			{
				Role:    string(api.MessageRoleUser),
				Content: pqtype.NullRawMessage{RawMessage: userContent, Valid: true},
			},
			{
				Role:    string(api.MessageRoleAssistant),
				Content: pqtype.NullRawMessage{RawMessage: assistantContent, Valid: true},
			},
			{
				Role:    string(api.MessageRoleTool),
				Content: pqtype.NullRawMessage{RawMessage: toolResults, Valid: true},
			},
		}

		prompt, err := chatMessagesToPrompt(messages)
		require.NoError(t, err)
		require.Len(t, prompt, 3, "no injection when all results present")
	})
}
