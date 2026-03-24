package cli //nolint:testpackage // Tests unexported chat stream helpers.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

type chatWatchWriters struct {
	stdout io.Writer
	stderr io.Writer
}

func (w chatWatchWriters) Write(p []byte) (int, error) {
	return w.stdout.Write(p)
}

func (w chatWatchWriters) Stderr() io.Writer {
	if w.stderr != nil {
		return w.stderr
	}
	return w.stdout
}

type chatStreamRenderState struct {
	printedInline bool
}

type chatStreamOutputMode int

const (
	chatStreamOutputModeText chatStreamOutputMode = iota
	chatStreamOutputModeJSON
)

func consumeChatStream(eventCh <-chan codersdk.ChatStreamEvent, out io.Writer, outputMode chatStreamOutputMode) error {
	if outputMode == chatStreamOutputModeJSON {
		encoder := json.NewEncoder(out)
		encoder.SetEscapeHTML(false)
		for event := range eventCh {
			if err := encoder.Encode(event); err != nil {
				return xerrors.Errorf("encode chat stream event: %w", err)
			}
		}
		return nil
	}

	errOut := chatWatchStderr(out)
	state := chatStreamRenderState{}
	for event := range eventCh {
		if err := renderChatStreamEvent(out, errOut, event, &state); err != nil {
			return xerrors.Errorf("render chat stream event: %w", err)
		}
	}

	if err := flushChatStreamInline(out, &state); err != nil {
		return xerrors.Errorf("flush chat stream output: %w", err)
	}

	return nil
}

func renderChatStreamEvent(
	out io.Writer,
	errOut io.Writer,
	event codersdk.ChatStreamEvent,
	state *chatStreamRenderState,
) error {
	switch event.Type {
	case codersdk.ChatStreamEventTypeMessagePart:
		if event.MessagePart == nil {
			return nil
		}
		if event.MessagePart.Part.Type != codersdk.ChatMessagePartTypeText {
			return nil
		}
		if event.MessagePart.Part.Text == "" {
			return nil
		}
		state.printedInline = true
		_, err := fmt.Fprint(out, event.MessagePart.Part.Text)
		return err
	case codersdk.ChatStreamEventTypeMessage:
		if event.Message != nil && !state.printedInline {
			text := chatMessageText(event.Message.Content)
			if text != "" {
				if _, err := fmt.Fprint(out, text); err != nil {
					return err
				}
				state.printedInline = true
			}
		}
		return flushChatStreamInline(out, state)
	case codersdk.ChatStreamEventTypeStatus:
		if err := flushChatStreamInline(out, state); err != nil {
			return err
		}
		if event.Status == nil {
			return nil
		}
		_, err := fmt.Fprintf(out, "[Status: %s]\n", event.Status.Status)
		return err
	case codersdk.ChatStreamEventTypeError:
		if err := flushChatStreamInline(out, state); err != nil {
			return err
		}
		if event.Error == nil {
			return nil
		}
		_, err := fmt.Fprintf(errOut, "[Error: %s]\n", event.Error.Message)
		return err
	case codersdk.ChatStreamEventTypeRetry:
		if err := flushChatStreamInline(out, state); err != nil {
			return err
		}
		if event.Retry == nil {
			return nil
		}
		_, err := fmt.Fprintf(out, "[Retry attempt %d after error: %s]\n", event.Retry.Attempt, event.Retry.Error)
		return err
	case codersdk.ChatStreamEventTypeQueueUpdate:
		return nil
	default:
		if err := flushChatStreamInline(out, state); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "[Event: %s]\n", event.Type)
		return err
	}
}

func flushChatStreamInline(out io.Writer, state *chatStreamRenderState) error {
	if !state.printedInline {
		return nil
	}
	state.printedInline = false
	_, err := fmt.Fprintln(out)
	return err
}

func chatWatchStderr(out io.Writer) io.Writer {
	type stderrWriter interface {
		Stderr() io.Writer
	}

	if writer, ok := out.(stderrWriter); ok {
		return writer.Stderr()
	}

	return out
}

func chatMessageText(parts []codersdk.ChatMessagePart) string {
	var builder strings.Builder
	for _, part := range parts {
		if part.Type != codersdk.ChatMessagePartTypeText {
			continue
		}
		_, _ = builder.WriteString(part.Text)
	}
	return builder.String()
}

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
