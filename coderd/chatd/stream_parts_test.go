package chatd

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"charm.land/fantasy"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestSDKChatMessage_ToolResultPartMetadata(t *testing.T) {
	t.Parallel()

	content, err := marshalToolResults([]ToolResultBlock{{
		ToolCallID: "call-3",
		ToolName:   toolExecute,
		Result: map[string]any{
			"output":    "completed",
			"exit_code": 17,
		},
	}})
	require.NoError(t, err)

	message := SDKChatMessage(database.ChatMessage{
		ID:        42,
		ChatID:    uuid.New(),
		CreatedAt: time.Now(),
		Role:      string(fantasy.MessageRoleTool),
		Content:   content,
		ToolCallID: sql.NullString{
			String: "call-3",
			Valid:  true,
		},
	})

	require.Len(t, message.Parts, 1)
	part := message.Parts[0]
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, part.Type)
	require.Equal(t, "call-3", part.ToolCallID)
	require.Equal(t, toolExecute, part.ToolName)
	require.NotEmpty(t, part.Result)
	require.NotNil(t, part.ResultMeta)
	require.Equal(t, "completed", part.ResultMeta.Output)
	require.NotNil(t, part.ResultMeta.ExitCode)
	require.Equal(t, 17, *part.ResultMeta.ExitCode)
}

func TestStreamManager_SnapshotBuffersOnlyMessageParts(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	manager := NewStreamManager(testutil.Logger(t))
	manager.StartStream(chatID)
	manager.Publish(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeStatus,
		Status: &codersdk.ChatStreamStatus{
			Status: codersdk.ChatStatusRunning,
		},
	})
	manager.Publish(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{
			Role: string(fantasy.MessageRoleAssistant),
			Part: codersdk.ChatMessagePart{
				Type: codersdk.ChatMessagePartTypeText,
				Text: "chunk",
			},
		},
	})
	manager.Publish(chatID, codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessage,
		Message: &codersdk.ChatMessage{
			ID: 1,
		},
	})

	snapshot, _, cancel := manager.Subscribe(chatID)
	defer cancel()

	require.Len(t, snapshot, 1)
	require.Equal(t, codersdk.ChatStreamEventTypeMessagePart, snapshot[0].Type)
	require.NotNil(t, snapshot[0].MessagePart)
	require.Equal(t, "chunk", snapshot[0].MessagePart.Part.Text)
}

func TestToolResultMetadata_ReadFileFields(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal([]ToolResultBlock{{
		ToolCallID: "call-4",
		ToolName:   toolReadFile,
		Result: map[string]any{
			"content":   "hello",
			"mime_type": "text/plain",
		},
	}})
	require.NoError(t, err)

	message := SDKChatMessage(database.ChatMessage{
		Role: string(fantasy.MessageRoleTool),
		Content: pqtype.NullRawMessage{
			RawMessage: raw,
			Valid:      true,
		},
	})
	require.Len(t, message.Parts, 1)
	require.NotNil(t, message.Parts[0].ResultMeta)
	require.Equal(t, "hello", message.Parts[0].ResultMeta.Content)
	require.Equal(t, "text/plain", message.Parts[0].ResultMeta.MimeType)
}
