package chatd

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"charm.land/fantasy"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
)

// replaceUnsupportedMedia replaces media that the model's route cannot take
// with short text notes, in user file parts and in tool results alike. It
// runs at prompt build so provider switches do not invalidate stored media,
// and it leaves the input unchanged because compaction can share the
// generation prompt.
func replaceUnsupportedMedia(
	ctx context.Context,
	logger slog.Logger,
	messages []fantasy.Message,
	model chatprovider.Model,
) []fantasy.Message {
	imageCap, hasImageCap := model.InlineImageCapBytes()
	// omissionNote returns the note that replaces the media, or "" to keep
	// it. unsupported is the note for a rejected media type.
	omissionNote := func(mediaType string, size int, unsupported string) string {
		if !model.AcceptsFilePartMediaType(mediaType) {
			return unsupported
		}
		if hasImageCap && size >= imageCap && chatfiles.IsRasterImageMediaType(mediaType) {
			return fmt.Sprintf("[image omitted: %d bytes exceeds the inline image limit of %d bytes]", size, imageCap)
		}
		return ""
	}

	replaced := 0
	out := make([]fantasy.Message, 0, len(messages))
	for _, msg := range messages {
		parts := make([]fantasy.MessagePart, 0, len(msg.Content))
		for _, part := range msg.Content {
			switch typed := part.(type) {
			case fantasy.FilePart:
				note := omissionNote(typed.MediaType, len(typed.Data), unsupportedAttachmentNote(typed.Filename, typed.MediaType))
				if note != "" {
					replaced++
					part = fantasy.TextPart{Text: note, ProviderOptions: typed.ProviderOptions}
				}
			case fantasy.ToolResultPart:
				media, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](typed.Output)
				if !ok {
					break
				}
				note := omissionNote(media.MediaType, base64DecodedLen(media.Data), unsupportedToolMediaNote(media.MediaType))
				if note != "" {
					replaced++
					// The note takes the place of the media, which providers
					// send before the accompanying text.
					if media.Text != "" {
						note += "\n\n" + media.Text
					}
					typed.Output = fantasy.ToolResultOutputContentText{Text: note}
					part = typed
				}
			}
			parts = append(parts, part)
		}
		msg.Content = parts
		out = append(out, msg)
	}
	if replaced > 0 {
		logger.Debug(ctx, "replaced unsupported media in prompt",
			slog.F("provider", model.Provider()),
			slog.F("configured_provider", model.RouteProvider()),
			slog.F("replaced_parts", replaced),
		)
	}
	return out
}

func unsupportedAttachmentNote(name, mediaType string) string {
	if name == "" {
		return fmt.Sprintf("[Attachment (%s) omitted: not supported by this model]", mediaType)
	}
	return fmt.Sprintf("[Attachment %q (%s) omitted: not supported by this model]", name, mediaType)
}

// unsupportedToolMediaNote is the note fantasy's providers write for tool
// result media they cannot send, so the model sees one wording whichever
// layer drops the media.
func unsupportedToolMediaNote(mediaType string) string {
	if mediaType == "" {
		mediaType = "file"
	}
	return fmt.Sprintf("The tool returned %s content, which cannot be displayed.", mediaType)
}

func base64DecodedLen(data string) int {
	return base64.RawStdEncoding.DecodedLen(len(strings.TrimRight(data, "=")))
}
