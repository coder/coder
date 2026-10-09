package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

func TestACPResultProjectionsCommit(t *testing.T) {
	t.Parallel()
	model := json.RawMessage(`{"response":"answer"}`)
	user := json.RawMessage(`{"messages":[{"role":"assistant","content":[{"type":"reasoning","text":"private reasoning"}]}]}`)
	response := chattool.WithUserResult(fantasy.NewTextResponse(string(model)), user)
	result := fantasy.ToolResultContent{ToolCallID: "wait", ToolName: "acp_wait_agent", Result: fantasy.ToolResultOutputContentText{Text: string(model)}, ClientMetadata: response.Metadata}
	call := fantasy.ToolCallContent{ToolCallID: "wait", ToolName: "acp_wait_agent", Input: `{"session_id":"x"}`}
	commit, err := buildCommitStepMessages(buildCommitStepMessagesInput{logger: slog.Make(), contentVersion: chatprompt.CurrentContentVersion, step: stepData{Content: []fantasy.Content{call, result}}})
	require.NoError(t, err)
	require.Len(t, commit.Messages, 3)
	require.Equal(t, database.ChatMessageVisibilityUser, commit.Messages[1].Visibility)
	require.Equal(t, database.ChatMessageVisibilityModel, commit.Messages[2].Visibility)
	require.Contains(t, string(commit.Messages[1].Content.RawMessage), "private reasoning")
	require.NotContains(t, string(commit.Messages[2].Content.RawMessage), "private reasoning")
	require.JSONEq(t, string(model), string(parseMessageParts(t, commit.Messages[2].Role, commit.Messages[2].Content)[0].Result))
	// Generation and compaction both consume this conversion of prompt rows.
	rows := make([]database.ChatMessage, 0, len(commit.Messages))
	for _, message := range commit.Messages {
		rows = append(rows, database.ChatMessage{Role: message.Role, Content: message.Content, Visibility: message.Visibility, ContentVersion: message.ContentVersion})
	}
	prompt, err := chatprompt.ConvertMessagesWithFiles(context.Background(), rows, nil, slog.Make(), nil, uuid.NullUUID{})
	require.NoError(t, err)
	encoded, err := json.Marshal(prompt)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private reasoning")
	require.Contains(t, string(encoded), "answer")
}
