package chatd

import (
	"context"
	"strings"

	"charm.land/fantasy"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

// replaceUnsupportedToolMedia rewrites tool result media that the model's
// transport rejects into the accompanying text plus an omission note, on a
// copy of messages. It runs on every prompt build, so media recorded under
// a more permissive provider stays replayable after a provider switch while
// the persisted result keeps its payload for the UI. displayProvider is the
// configured provider named in the note.
func replaceUnsupportedToolMedia(
	ctx context.Context,
	logger slog.Logger,
	messages []fantasy.Message,
	model chatprovider.Model,
	displayProvider string,
) []fantasy.Message {
	replaced := 0
	out := make([]fantasy.Message, 0, len(messages))
	for _, msg := range messages {
		parts := make([]fantasy.MessagePart, 0, len(msg.Content))
		for _, part := range msg.Content {
			result, ok := part.(fantasy.ToolResultPart)
			if !ok {
				parts = append(parts, part)
				continue
			}
			media, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](result.Output)
			if !ok {
				parts = append(parts, part)
				continue
			}
			note, omit := chatprovider.ToolResultMediaOmission(model.Provider(), displayProvider, media.MediaType, base64DecodedLen(media.Data))
			if !omit {
				parts = append(parts, part)
				continue
			}
			replaced++
			text := media.Text
			if text != "" {
				text += "\n"
			}
			result.Output = fantasy.ToolResultOutputContentText{Text: text + note}
			parts = append(parts, result)
		}
		msg.Content = parts
		out = append(out, msg)
	}
	if replaced > 0 {
		logger.Debug(ctx, "replaced unsupported tool result media in prompt",
			slog.F("provider", model.Provider()),
			slog.F("replaced_parts", replaced),
		)
	}
	return out
}

// base64DecodedLen returns the byte length of standard base64 data
// without decoding it.
func base64DecodedLen(data string) int {
	n := len(data) / 4 * 3
	if strings.HasSuffix(data, "==") {
		return n - 2
	}
	if strings.HasSuffix(data, "=") {
		return n - 1
	}
	return n
}
