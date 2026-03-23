package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
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

func (r *RootCmd) chatsWatch() *serpent.Command {
	var (
		after      int64
		outputMode string
	)

	return &serpent.Command{
		Use:        "watch <chat-id>",
		Short:      "Watch a chat for live updates.",
		Middleware: serpent.RequireNArgs(1),
		Options: serpent.OptionSet{
			{
				Name:        "after",
				Flag:        "after",
				Description: "Only stream messages created after the given message ID.",
				Value:       serpent.Int64Of(&after),
			},
			{
				Name:          "output",
				Flag:          "output",
				FlagShorthand: "o",
				Default:       "text",
				Description:   "Output format.",
				Value:         serpent.EnumOf(&outputMode, "text", "json"),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			chatID, err := parseChatID(inv)
			if err != nil {
				return err
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			expClient := codersdk.NewExperimentalClient(client)

			var afterID *int64
			if after > 0 {
				afterID = &after
			}

			return watchChat(
				inv.Context(),
				expClient,
				chatID,
				afterID,
				chatWatchWriters{stdout: inv.Stdout, stderr: inv.Stderr},
				outputMode == "json",
			)
		},
	}
}

// watchChat runs the streaming loop. It's used by the watch command directly
// and by start/send with --follow.
//
//nolint:revive // The helper mirrors the CLI's text-vs-JSON follow mode.
func watchChat(
	ctx context.Context,
	client *codersdk.ExperimentalClient,
	chatID uuid.UUID,
	afterID *int64,
	out io.Writer,
	jsonMode bool,
) error {
	eventCh, closer, err := client.StreamChat(ctx, chatID, &codersdk.StreamChatOptions{AfterID: afterID})
	if err != nil {
		return xerrors.Errorf("streaming chat: %w", err)
	}
	defer closer.Close()

	outputMode := chatStreamOutputModeText
	if jsonMode {
		outputMode = chatStreamOutputModeJSON
	}

	return consumeChatStream(eventCh, out, outputMode)
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
