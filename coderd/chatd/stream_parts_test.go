package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"iter"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"go.jetify.com/ai/api"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type scriptedModel struct {
	streamErr      error
	streamEvents   []api.StreamEvent
	generateBlocks []api.ContentBlock
}

func (m *scriptedModel) ProviderName() string {
	return "fake"
}

func (m *scriptedModel) ModelID() string {
	return "fake"
}

func (m *scriptedModel) SupportedUrls() []api.SupportedURL {
	return nil
}

func (m *scriptedModel) Generate(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.Response, error) {
	return &api.Response{Content: m.generateBlocks}, nil
}

func (m *scriptedModel) Stream(_ context.Context, _ []api.Message, _ api.CallOptions) (*api.StreamResponse, error) {
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	return &api.StreamResponse{
		Stream: iter.Seq[api.StreamEvent](func(yield func(api.StreamEvent) bool) {
			for _, event := range m.streamEvents {
				if !yield(event) {
					return
				}
			}
		}),
	}, nil
}

func TestStreamChatResponse_EmitsMessageParts(t *testing.T) {
	t.Parallel()

	model := &scriptedModel{
		streamEvents: []api.StreamEvent{
			&api.TextDeltaEvent{TextDelta: "hel"},
			&api.TextDeltaEvent{TextDelta: "lo"},
			&api.ReasoningEvent{TextDelta: "thinking"},
			&api.ToolCallDeltaEvent{
				ToolCallID: "call-1",
				ToolName:   toolExecute,
				ArgsDelta:  []byte(`{"command":"echo`),
			},
			&api.ToolCallDeltaEvent{
				ToolCallID: "call-1",
				ToolName:   toolExecute,
				ArgsDelta:  []byte(` hi"}`),
			},
			&api.ToolCallEvent{
				ToolCallID: "call-1",
				ToolName:   toolExecute,
				Args:       json.RawMessage(`{"command":"echo hi"}`),
			},
			&api.FinishEvent{Usage: api.Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}},
		},
	}

	var events []codersdk.ChatStreamEvent
	result, err := streamChatResponse(
		context.Background(),
		model,
		nil,
		nil,
		func(event codersdk.ChatStreamEvent) {
			events = append(events, event)
		},
	)
	require.NoError(t, err)
	require.Len(t, events, 6)
	require.Len(t, result.Content, 3)

	require.Equal(t, codersdk.ChatStreamEventTypeMessagePart, events[0].Type)
	require.NotNil(t, events[0].MessagePart)
	require.Equal(t, string(api.MessageRoleAssistant), events[0].MessagePart.Role)
	require.Equal(t, codersdk.ChatMessagePartTypeText, events[0].MessagePart.Part.Type)
	require.Equal(t, "hel", events[0].MessagePart.Part.Text)

	require.Equal(t, codersdk.ChatMessagePartTypeReasoning, events[2].MessagePart.Part.Type)
	require.Equal(t, "thinking", events[2].MessagePart.Part.Text)

	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, events[3].MessagePart.Part.Type)
	require.Equal(t, "call-1", events[3].MessagePart.Part.ToolCallID)
	require.Equal(t, toolExecute, events[3].MessagePart.Part.ToolName)
	require.Equal(t, `{"command":"echo`, events[3].MessagePart.Part.ArgsDelta)

	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, events[5].MessagePart.Part.Type)
	require.Equal(t, json.RawMessage(`{"command":"echo hi"}`), events[5].MessagePart.Part.Args)
}

func TestStreamChatResponse_UnsupportedStreamingStillEmitsParts(t *testing.T) {
	t.Parallel()

	model := &scriptedModel{
		streamErr: api.NewUnsupportedFunctionalityError("stream", ""),
		generateBlocks: []api.ContentBlock{
			&api.TextBlock{Text: "fallback"},
			&api.ToolCallBlock{
				ToolCallID: "call-2",
				ToolName:   toolReadFile,
				Args:       json.RawMessage(`{"path":"README.md"}`),
			},
		},
	}

	var events []codersdk.ChatStreamEvent
	result, err := streamChatResponse(
		context.Background(),
		model,
		nil,
		nil,
		func(event codersdk.ChatStreamEvent) {
			events = append(events, event)
		},
	)
	require.NoError(t, err)
	require.Len(t, result.Content, 2)
	require.Len(t, events, 2)

	require.Equal(t, codersdk.ChatStreamEventTypeMessagePart, events[0].Type)
	require.NotNil(t, events[0].MessagePart)
	require.Equal(t, codersdk.ChatMessagePartTypeText, events[0].MessagePart.Part.Type)
	require.Equal(t, "fallback", events[0].MessagePart.Part.Text)

	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, events[1].MessagePart.Part.Type)
	require.Equal(t, "call-2", events[1].MessagePart.Part.ToolCallID)
	require.Equal(t, toolReadFile, events[1].MessagePart.Part.ToolName)
}

func TestSDKChatMessage_ToolResultPartMetadata(t *testing.T) {
	t.Parallel()

	content, err := marshalToolResults([]api.ToolResultBlock{{
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
		Role:      string(api.MessageRoleTool),
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
			Role: string(api.MessageRoleAssistant),
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

	raw, err := json.Marshal([]api.ToolResultBlock{{
		ToolCallID: "call-4",
		ToolName:   toolReadFile,
		Result: map[string]any{
			"content":   "hello",
			"mime_type": "text/plain",
		},
	}})
	require.NoError(t, err)

	message := SDKChatMessage(database.ChatMessage{
		Role: string(api.MessageRoleTool),
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
