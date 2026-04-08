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

type chatWatchWriters struct{ stdout, stderr io.Writer }

func (w chatWatchWriters) Write(p []byte) (int, error) { return w.stdout.Write(p) }

func (w chatWatchWriters) Stderr() io.Writer {
	if w.stderr != nil {
		return w.stderr
	}
	return w.stdout
}

func consumeChatStream(eventCh <-chan codersdk.ChatStreamEvent, out io.Writer) error {
	errOut := out
	if writer, ok := out.(interface{ Stderr() io.Writer }); ok {
		errOut = writer.Stderr()
	}

	printedInline := false
	flush := func() error {
		if !printedInline {
			return nil
		}
		printedInline = false
		_, err := fmt.Fprintln(out)
		return err
	}

	printLine := func(dst io.Writer, format string, args ...any) error {
		if err := flush(); err != nil {
			return err
		}
		_, err := fmt.Fprintf(dst, format, args...)
		return err
	}

	for event := range eventCh {
		var err error
		switch event.Type {
		case codersdk.ChatStreamEventTypeMessagePart:
			if part := event.MessagePart; part != nil &&
				part.Part.Type == codersdk.ChatMessagePartTypeText && part.Part.Text != "" {
				printedInline = true
				_, err = fmt.Fprint(out, part.Part.Text)
			}
		case codersdk.ChatStreamEventTypeMessage:
			if message := event.Message; message != nil && !printedInline {
				for _, part := range message.Content {
					if part.Type != codersdk.ChatMessagePartTypeText || part.Text == "" {
						continue
					}
					printedInline = true
					if _, err = fmt.Fprint(out, part.Text); err != nil {
						break
					}
				}
			}
			if err == nil {
				err = flush()
			}
		case codersdk.ChatStreamEventTypeStatus:
			if event.Status == nil {
				err = flush()
				break
			}
			err = printLine(out, "[Status: %s]\n", event.Status.Status)
		case codersdk.ChatStreamEventTypeError:
			if event.Error == nil {
				err = flush()
				break
			}
			err = printLine(errOut, "[Error: %s]\n", event.Error.Message)
		case codersdk.ChatStreamEventTypeRetry:
			if event.Retry == nil {
				err = flush()
				break
			}
			err = printLine(out, "[Retry attempt %d after error: %s]\n", event.Retry.Attempt, event.Retry.Error)
		case codersdk.ChatStreamEventTypeQueueUpdate:
		default:
			err = printLine(out, "[Event: %s]\n", event.Type)
		}
		if err != nil {
			return xerrors.Errorf("render chat stream event: %w", err)
		}
	}

	if err := flush(); err != nil {
		return xerrors.Errorf("flush chat stream output: %w", err)
	}
	return nil
}

func TestConsumeChatStreamText(t *testing.T) {
	t.Parallel()

	events := make(chan codersdk.ChatStreamEvent, 7)
	for _, event := range []codersdk.ChatStreamEvent{
		{Type: codersdk.ChatStreamEventTypeMessagePart, MessagePart: &codersdk.ChatStreamMessagePart{Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: "Hello"}}},
		{Type: codersdk.ChatStreamEventTypeMessagePart, MessagePart: &codersdk.ChatStreamMessagePart{Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeToolCall, Text: "ignored"}}},
		{Type: codersdk.ChatStreamEventTypeMessagePart, MessagePart: &codersdk.ChatStreamMessagePart{Role: codersdk.ChatMessageRoleAssistant, Part: codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeText, Text: " world"}}},
		{Type: codersdk.ChatStreamEventTypeMessage, Message: &codersdk.ChatMessage{ID: 1, ChatID: uuid.New(), Role: codersdk.ChatMessageRoleAssistant, Content: []codersdk.ChatMessagePart{{Type: codersdk.ChatMessagePartTypeText, Text: "Hello world"}}}},
		{Type: codersdk.ChatStreamEventTypeStatus, Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatusRunning}},
		{Type: codersdk.ChatStreamEventTypeRetry, Retry: &codersdk.ChatStreamRetry{Attempt: 2, Error: "rate limited"}},
		{Type: codersdk.ChatStreamEventTypeError, Error: &codersdk.ChatStreamError{Message: "boom"}},
	} {
		events <- event
	}
	close(events)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := consumeChatStream(events, chatWatchWriters{stdout: &stdout, stderr: &stderr})
	require.NoError(t, err)
	require.Equal(t, "Hello world\n[Status: running]\n[Retry attempt 2 after error: rate limited]\n", stdout.String())
	require.Equal(t, "[Error: boom]\n", stderr.String())
}

func TestConsumeChatStreamJSON(t *testing.T) {
	t.Parallel()

	chatID := uuid.New()
	events := make(chan codersdk.ChatStreamEvent, 2)
	for _, event := range []codersdk.ChatStreamEvent{
		{Type: codersdk.ChatStreamEventTypeStatus, ChatID: chatID, Status: &codersdk.ChatStreamStatus{Status: codersdk.ChatStatusPending}},
		{Type: codersdk.ChatStreamEventTypeError, ChatID: chatID, Error: &codersdk.ChatStreamError{Message: "failed"}},
	} {
		events <- event
	}
	close(events)

	var stdout bytes.Buffer

	err := func() error {
		encoder := json.NewEncoder(&stdout)
		encoder.SetEscapeHTML(false)
		for event := range events {
			if err := encoder.Encode(event); err != nil {
				return xerrors.Errorf("encode chat stream event: %w", err)
			}
		}
		return nil
	}()
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
