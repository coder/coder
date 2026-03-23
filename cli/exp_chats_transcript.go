package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) chatsTranscript() *serpent.Command {
	var (
		outputMode   string
		includeTools bool
		outputFile   string
	)

	return &serpent.Command{
		Use:        "transcript <chat-id>",
		Short:      "Show the transcript of a chat.",
		Middleware: serpent.RequireNArgs(1),
		Options: serpent.OptionSet{
			{
				Name:          "output",
				Flag:          "output",
				FlagShorthand: "o",
				Default:       "text",
				Description:   "Output format.",
				Value:         serpent.EnumOf(&outputMode, "text", "json"),
			},
			{
				Name:        "include-tools",
				Flag:        "include-tools",
				Default:     "false",
				Description: "Include tool call and tool result parts in text output.",
				Value:       serpent.BoolOf(&includeTools),
			},
			{
				Name:        "output-file",
				Flag:        "output-file",
				Description: "Write the transcript to a file instead of stdout.",
				Value:       serpent.StringOf(&outputFile),
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

			allMessages, err := fetchAllChatMessages(inv.Context(), client, chatID)
			if err != nil {
				return xerrors.Errorf("get chat messages: %w", err)
			}

			var output []byte
			switch outputMode {
			case "json":
				output, err = json.Marshal(allMessages)
				if err != nil {
					return xerrors.Errorf("marshal chat messages: %w", err)
				}
				output = append(output, '\n')
			case "text":
				toolMode := transcriptToolModeOmit
				if includeTools {
					toolMode = transcriptToolModeInclude
				}
				output = []byte(renderChatTranscript(allMessages, toolMode))
			default:
				return xerrors.Errorf("unsupported output format %q", outputMode)
			}

			if outputFile != "" {
				if err := os.WriteFile(outputFile, output, 0o600); err != nil {
					return xerrors.Errorf("write transcript to file: %w", err)
				}
				return nil
			}

			_, err = inv.Stdout.Write(output)
			return err
		},
	}
}

func fetchAllChatMessages(ctx context.Context, client *codersdk.Client, chatID uuid.UUID) ([]codersdk.ChatMessage, error) {
	var (
		allMessages []codersdk.ChatMessage
		opts        *codersdk.ChatMessagesPaginationOptions
	)

	for {
		resp, err := client.GetChatMessages(ctx, chatID, opts)
		if err != nil {
			return nil, err
		}

		allMessages = append(allMessages, resp.Messages...)
		if !resp.HasMore || len(resp.Messages) == 0 {
			break
		}

		opts = &codersdk.ChatMessagesPaginationOptions{
			BeforeID: resp.Messages[len(resp.Messages)-1].ID,
		}
	}

	slices.SortStableFunc(allMessages, func(a, b codersdk.ChatMessage) int {
		switch {
		case a.CreatedAt.Before(b.CreatedAt):
			return -1
		case a.CreatedAt.After(b.CreatedAt):
			return 1
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})

	return allMessages, nil
}

func renderChatTranscript(messages []codersdk.ChatMessage, toolMode transcriptToolMode) string {
	var builder strings.Builder
	first := true

	for _, message := range messages {
		if message.Role == codersdk.ChatMessageRoleSystem {
			continue
		}

		body := renderChatTranscriptMessage(message.Content, toolMode)
		if body == "" {
			continue
		}

		if !first {
			_, _ = builder.WriteString("\n")
		}
		first = false

		_, _ = fmt.Fprintf(
			&builder,
			"=== %s (%s) ===\n%s\n",
			titleCase(string(message.Role)),
			message.CreatedAt.Format(time.RFC3339),
			body,
		)
	}

	return builder.String()
}

type transcriptToolMode int

const (
	transcriptToolModeOmit transcriptToolMode = iota
	transcriptToolModeInclude
)

func renderChatTranscriptMessage(parts []codersdk.ChatMessagePart, toolMode transcriptToolMode) string {
	var (
		lines       []string
		textBuilder strings.Builder
	)

	flushText := func() {
		if textBuilder.Len() == 0 {
			return
		}
		lines = append(lines, textBuilder.String())
		textBuilder.Reset()
	}

	for _, part := range parts {
		switch part.Type {
		case codersdk.ChatMessagePartTypeText, codersdk.ChatMessagePartTypeReasoning:
			_, _ = textBuilder.WriteString(part.Text)
		case codersdk.ChatMessagePartTypeToolCall:
			if toolMode == transcriptToolModeOmit {
				continue
			}
			flushText()
			lines = append(lines, formatTranscriptToolCall(part))
		case codersdk.ChatMessagePartTypeToolResult:
			if toolMode == transcriptToolModeOmit {
				continue
			}
			flushText()
			lines = append(lines, formatTranscriptToolResult(part))
		}
	}

	flushText()
	return strings.Join(lines, "\n")
}

func formatTranscriptToolCall(part codersdk.ChatMessagePart) string {
	args := compactTranscriptJSON(part.Args)
	if args == "" {
		return fmt.Sprintf("[Tool Call: %s()]", part.ToolName)
	}
	return fmt.Sprintf("[Tool Call: %s(%s)]", part.ToolName, args)
}

func formatTranscriptToolResult(part codersdk.ChatMessagePart) string {
	result := compactTranscriptJSON(part.Result)
	if result == "" {
		result = "null"
	}
	return fmt.Sprintf("[Tool Result: %s → %s]", part.ToolName, result)
}

func compactTranscriptJSON(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}

	var builder bytes.Buffer
	if err := json.Compact(&builder, trimmed); err == nil {
		return builder.String()
	}

	return string(trimmed)
}

func titleCase(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
