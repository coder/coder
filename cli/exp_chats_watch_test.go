package cli //nolint:testpackage // Tests unexported chat stream helpers.

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestConsumeChatStreamText(t *testing.T) {
	t.Parallel()

	events := make(chan codersdk.ChatStreamEvent, 7)
	events <- codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{
			Role: codersdk.ChatMessageRoleAssistant,
			Part: codersdk.ChatMessagePart{
				Type: codersdk.ChatMessagePartTypeText,
				Text: "Hello",
			},
		},
	}
	events <- codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{
			Role: codersdk.ChatMessageRoleAssistant,
			Part: codersdk.ChatMessagePart{
				Type: codersdk.ChatMessagePartTypeToolCall,
				Text: "ignored",
			},
		},
	}
	events <- codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessagePart,
		MessagePart: &codersdk.ChatStreamMessagePart{
			Role: codersdk.ChatMessageRoleAssistant,
			Part: codersdk.ChatMessagePart{
				Type: codersdk.ChatMessagePartTypeText,
				Text: " world",
			},
		},
	}
	events <- codersdk.ChatStreamEvent{
		Type: codersdk.ChatStreamEventTypeMessage,
		Message: &codersdk.ChatMessage{
			ID:     1,
			ChatID: uuid.New(),
			Role:   codersdk.ChatMessageRoleAssistant,
			Content: []codersdk.ChatMessagePart{{
				Type: codersdk.ChatMessagePartTypeText,
				Text: "Hello world",
			}},
		},
	}
	events <- codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeStatus,
		Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatusRunning},
	}
	events <- codersdk.ChatStreamEvent{
		Type:  codersdk.ChatStreamEventTypeRetry,
		Retry: &codersdk.ChatStreamRetry{Attempt: 2, Error: "rate limited"},
	}
	events <- codersdk.ChatStreamEvent{
		Type:  codersdk.ChatStreamEventTypeError,
		Error: &codersdk.ChatStreamError{Message: "boom"},
	}
	close(events)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := consumeChatStream(events, chatWatchWriters{stdout: &stdout, stderr: &stderr}, chatStreamOutputModeText)
	require.NoError(t, err)
	require.Equal(t, "Hello world\n[Status: running]\n[Retry attempt 2 after error: rate limited]\n", stdout.String())
	require.Equal(t, "[Error: boom]\n", stderr.String())
}

func TestConsumeChatStreamJSON(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	events := make(chan codersdk.ChatStreamEvent, 2)
	events <- codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeStatus,
		ChatID: chatID,
		Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatusPending},
	}
	events <- codersdk.ChatStreamEvent{
		Type:   codersdk.ChatStreamEventTypeError,
		ChatID: chatID,
		Error:  &codersdk.ChatStreamError{Message: "failed"},
	}
	close(events)

	var stdout bytes.Buffer
	err := consumeChatStream(events, &stdout, chatStreamOutputModeJSON)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	require.Len(t, lines, 2)

	var statusEvent codersdk.ChatStreamEvent
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &statusEvent))
	require.Equal(t, codersdk.ChatStreamEventTypeStatus, statusEvent.Type)
	require.NotNil(t, statusEvent.Status)
	require.Equal(t, codersdk.ChatStatusPending, statusEvent.Status.Status)

	var errorEvent codersdk.ChatStreamEvent
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &errorEvent))
	require.Equal(t, codersdk.ChatStreamEventTypeError, errorEvent.Type)
	require.NotNil(t, errorEvent.Error)
	require.Equal(t, "failed", errorEvent.Error.Message)
}
