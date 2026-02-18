package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"iter"
	"testing"
	"time"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type scriptedModel struct {
	streamErr      error
	streamEvents   []fantasy.StreamPart
	generateBlocks []fantasy.Content
	streamCall     *fantasy.Call
	generateCall   *fantasy.Call
}

func (m *scriptedModel) Provider() string {
	return "fake"
}

func (m *scriptedModel) Model() string {
	return "fake"
}

func (m *scriptedModel) Generate(_ context.Context, call fantasy.Call) (*fantasy.Response, error) {
	captured := call
	m.generateCall = &captured
	return &fantasy.Response{Content: m.generateBlocks}, nil
}

func (m *scriptedModel) Stream(_ context.Context, call fantasy.Call) (fantasy.StreamResponse, error) {
	captured := call
	m.streamCall = &captured
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	return iter.Seq[fantasy.StreamPart](func(yield func(fantasy.StreamPart) bool) {
		for _, event := range m.streamEvents {
			if !yield(event) {
				return
			}
		}
	}), nil
}

func (*scriptedModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return nil, xerrors.New("not implemented")
}

func (*scriptedModel) StreamObject(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error) {
	return nil, xerrors.New("not implemented")
}

func TestStreamChatResponse_EmitsMessageParts(t *testing.T) {
	t.Parallel()

	model := &scriptedModel{
		streamEvents: []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "hel"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "lo"},
			{Type: fantasy.StreamPartTypeReasoningDelta, ID: "reasoning-1", Delta: "thinking"},
			{
				Type:         fantasy.StreamPartTypeToolInputDelta,
				ID:           "call-1",
				ToolCallName: toolExecute,
				Delta:        `{"command":"echo`,
			},
			{
				Type:         fantasy.StreamPartTypeToolInputDelta,
				ID:           "call-1",
				ToolCallName: toolExecute,
				Delta:        ` hi"}`,
			},
			{
				Type:          fantasy.StreamPartTypeToolCall,
				ID:            "call-1",
				ToolCallName:  toolExecute,
				ToolCallInput: `{"command":"echo hi"}`,
			},
			{
				Type:  fantasy.StreamPartTypeFinish,
				Usage: fantasy.Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
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
	require.Len(t, events, 6)
	require.Len(t, result.Content, 3)
	require.NotNil(t, model.streamCall)
	require.NotNil(t, model.streamCall.ToolChoice)
	require.Equal(t, fantasy.ToolChoiceNone, *model.streamCall.ToolChoice)

	require.Equal(t, codersdk.ChatStreamEventTypeMessagePart, events[0].Type)
	require.NotNil(t, events[0].MessagePart)
	require.Equal(t, string(fantasy.MessageRoleAssistant), events[0].MessagePart.Role)
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

func TestStreamChatResponse_EmitsToolResultPartFromStreamChunk(t *testing.T) {
	t.Parallel()

	model := &scriptedModel{
		streamEvents: []fantasy.StreamPart{
			{
				Type:         fantasy.StreamPartTypeToolResult,
				ID:           "call-5",
				ToolCallName: toolExecute,
				Delta:        `{"output":"done","exit_code":0}`,
			},
			{
				Type:  fantasy.StreamPartTypeFinish,
				Usage: fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
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
	require.Len(t, events, 1)
	require.Len(t, result.Content, 1)

	require.Equal(t, codersdk.ChatStreamEventTypeMessagePart, events[0].Type)
	require.NotNil(t, events[0].MessagePart)
	require.Equal(t, string(fantasy.MessageRoleAssistant), events[0].MessagePart.Role)
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, events[0].MessagePart.Part.Type)
	require.Equal(t, "call-5", events[0].MessagePart.Part.ToolCallID)
	require.Equal(t, toolExecute, events[0].MessagePart.Part.ToolName)
	require.JSONEq(t, `{"output":"done","exit_code":0}`, string(events[0].MessagePart.Part.Result))
	require.NotNil(t, events[0].MessagePart.Part.ResultMeta)
	require.Equal(t, "done", events[0].MessagePart.Part.ResultMeta.Output)
	require.NotNil(t, events[0].MessagePart.Part.ResultMeta.ExitCode)
	require.Equal(t, 0, *events[0].MessagePart.Part.ResultMeta.ExitCode)

	toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](result.Content[0])
	require.True(t, ok)
	require.Equal(t, "call-5", toolResult.ToolCallID)
	require.Equal(t, toolExecute, toolResult.ToolName)

	output, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](toolResult.Result)
	require.True(t, ok)
	require.Equal(t, `{"output":"done","exit_code":0}`, output.Text)
}

func TestStreamChatResponse_UnsupportedStreamingReturnsError(t *testing.T) {
	t.Parallel()

	model := &scriptedModel{
		streamErr: xerrors.New("stream is not supported"),
		generateBlocks: []fantasy.Content{
			fantasy.TextContent{Text: "fallback"},
			fantasy.ToolCallContent{
				ToolCallID: "call-2",
				ToolName:   toolReadFile,
				Input:      `{"path":"README.md"}`,
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
	require.Error(t, err)
	require.Empty(t, result.Content)
	require.Empty(t, events)
	require.NotNil(t, model.streamCall)
	require.NotNil(t, model.streamCall.ToolChoice)
	require.Equal(t, fantasy.ToolChoiceNone, *model.streamCall.ToolChoice)
	require.Nil(t, model.generateCall)
}

func TestStreamChatResponse_StreamErrorFinalizesToolInputDeltas(t *testing.T) {
	t.Parallel()

	model := &scriptedModel{
		streamEvents: []fantasy.StreamPart{
			{
				Type:         fantasy.StreamPartTypeToolInputStart,
				ID:           "call-error-1",
				ToolCallName: toolReadFile,
			},
			{
				Type:         fantasy.StreamPartTypeToolInputDelta,
				ID:           "call-error-1",
				ToolCallName: toolReadFile,
				Delta:        `{"path":"README.md"}`,
			},
			{
				Type:  fantasy.StreamPartTypeError,
				Error: xerrors.New("stream failed"),
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
	require.Error(t, err)
	var streamErr *streamErrorReported
	require.ErrorAs(t, err, &streamErr)

	require.Len(t, result.Content, 1)
	toolCall, ok := fantasy.AsContentType[fantasy.ToolCallContent](result.Content[0])
	require.True(t, ok)
	require.Equal(t, "call-error-1", toolCall.ToolCallID)
	require.Equal(t, toolReadFile, toolCall.ToolName)
	require.JSONEq(t, `{"path":"README.md"}`, toolCall.Input)

	require.Len(t, events, 3)
	require.Equal(t, codersdk.ChatStreamEventTypeMessagePart, events[0].Type)
	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, events[0].MessagePart.Part.Type)
	require.Equal(t, `{"path":"README.md"}`, events[0].MessagePart.Part.ArgsDelta)

	require.Equal(t, codersdk.ChatStreamEventTypeMessagePart, events[1].Type)
	require.Equal(t, codersdk.ChatMessagePartTypeToolCall, events[1].MessagePart.Part.Type)
	require.Equal(t, json.RawMessage(`{"path":"README.md"}`), events[1].MessagePart.Part.Args)

	require.Equal(t, codersdk.ChatStreamEventTypeError, events[2].Type)
	require.NotNil(t, events[2].Error)
	require.Contains(t, events[2].Error.Message, "stream failed")
}

func TestStreamChatResponse_SetsToolChoiceAutoWhenToolsProvided(t *testing.T) {
	t.Parallel()

	model := &scriptedModel{
		streamEvents: []fantasy.StreamPart{
			{
				Type:  fantasy.StreamPartTypeFinish,
				Usage: fantasy.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
			},
		},
	}

	_, err := streamChatResponse(
		context.Background(),
		model,
		nil,
		[]fantasy.Tool{
			fantasy.FunctionTool{Name: "noop"},
		},
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, model.streamCall)
	require.NotNil(t, model.streamCall.ToolChoice)
	require.Equal(t, fantasy.ToolChoiceAuto, *model.streamCall.ToolChoice)
}

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
