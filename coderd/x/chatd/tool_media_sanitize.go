package chatd

import (
	"context"
	"encoding/base64"
	"slices"
	"strings"

	"charm.land/fantasy"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

// Filter at prompt build so provider switches do not invalidate stored media.
// Leave the input unchanged because compaction can share the generation prompt.
func replaceUnsupportedToolMedia(
	ctx context.Context,
	logger slog.Logger,
	messages []fantasy.Message,
	model chatprovider.Model,
	configuredProvider string,
) []fantasy.Message {
	replaced := 0
	provider := model.Provider()
	out := messages
	for i, msg := range messages {
		var parts []fantasy.MessagePart
		for j, part := range msg.Content {
			result, ok := part.(fantasy.ToolResultPart)
			if !ok {
				continue
			}
			media, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](result.Output)
			if !ok {
				continue
			}
			note, omit := chatprovider.ToolResultMediaOmission(provider, configuredProvider, media.MediaType, base64DecodedLen(media.Data))
			if !omit {
				continue
			}
			if replaced == 0 {
				out = slices.Clone(messages)
			}
			if parts == nil {
				parts = slices.Clone(msg.Content)
			}
			replaced++
			text := media.Text
			if text != "" {
				text += "\n"
			}
			result.Output = fantasy.ToolResultOutputContentText{Text: text + note}
			parts[j] = result
		}
		if parts != nil {
			out[i].Content = parts
		}
	}
	if replaced > 0 {
		logger.Debug(ctx, "replaced unsupported tool result media in prompt",
			slog.F("provider", provider),
			slog.F("replaced_parts", replaced),
		)
	}
	return out
}

func base64DecodedLen(data string) int {
	return base64.RawStdEncoding.DecodedLen(len(strings.TrimRight(data, "=")))
}
