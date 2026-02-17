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
