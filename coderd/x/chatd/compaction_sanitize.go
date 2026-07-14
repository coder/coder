package chatd

import (
	"context"
	"fmt"

	"charm.land/fantasy"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatsanitize"
)

// sameCompactionProviderIdentity reports whether the chat and compaction
// override models share a provider instance. Configs without an
// AIProviderID compare as different (fail closed).
func sameCompactionProviderIdentity(chatConfig, overrideConfig database.ChatModelConfig) bool {
	return chatConfig.AIProviderID.Valid && overrideConfig.AIProviderID.Valid &&
		chatConfig.AIProviderID.UUID == overrideConfig.AIProviderID.UUID
}

// sanitizeCompactionPrompt adapts a prompt built for the chat model to a
// differing compaction model. The input messages are never mutated; the
// assistant generation keeps using the original prompt.
func sanitizeCompactionPrompt(
	ctx context.Context,
	logger slog.Logger,
	prompt []fantasy.Message,
	compactionModel fantasy.LanguageModel,
	chatConfig database.ChatModelConfig,
	overrideConfig database.ChatModelConfig,
) []fantasy.Message {
	messages := prompt
	if !sameCompactionProviderIdentity(chatConfig, overrideConfig) {
		messages = flattenProviderExecutedToolParts(ctx, logger, messages)
	}
	messages = replaceUnsupportedFileParts(ctx, logger, messages, func(mediaType string) bool {
		return chatprovider.AcceptsFilePartMediaType(
			compactionModel.Provider(),
			compactionModel.Model(),
			mediaType,
		)
	})
	sanitized, stats := chatsanitize.SanitizeAnthropicProviderToolHistory(
		compactionModel.Provider(),
		messages,
	)
	chatsanitize.LogAnthropicProviderToolSanitization(
		ctx,
		logger,
		"compaction_prompt",
		compactionModel.Provider(),
		compactionModel.Model(),
		stats,
	)
	return sanitized
}

// flattenProviderExecutedToolParts rewrites provider-executed tool calls
// and results in assistant messages into text parts on a copy of messages,
// keeping their content while shedding provider-specific wire shapes other
// providers reject on replay. Provider-executed parts outside assistant
// messages are anomalous and dropped, since a text part is not valid
// tool-message content everywhere; messages emptied by the drop are removed.
func flattenProviderExecutedToolParts(
	ctx context.Context,
	logger slog.Logger,
	messages []fantasy.Message,
) []fantasy.Message {
	flattened := 0
	dropped := 0
	// Tool names live on the call part only; results reference the call by ID.
	toolNamesByCallID := make(map[string]string)
	out := make([]fantasy.Message, 0, len(messages))
	for _, msg := range messages {
		flattenToText := msg.Role == fantasy.MessageRoleAssistant
		parts := make([]fantasy.MessagePart, 0, len(msg.Content))
		for _, part := range msg.Content {
			switch typed := part.(type) {
			case fantasy.ToolCallPart:
				if typed.ProviderExecuted {
					if !flattenToText {
						dropped++
						continue
					}
					toolNamesByCallID[typed.ToolCallID] = typed.ToolName
					flattened++
					parts = append(parts, fantasy.TextPart{
						Text: fmt.Sprintf("[Server tool call: %s] %s", typed.ToolName, typed.Input),
					})
					continue
				}
			case fantasy.ToolResultPart:
				if typed.ProviderExecuted {
					if !flattenToText {
						dropped++
						continue
					}
					flattened++
					parts = append(parts, fantasy.TextPart{
						Text: fmt.Sprintf(
							"[Server tool result: %s] %s",
							toolNamesByCallID[typed.ToolCallID],
							stringifyToolResultOutput(typed.Output),
						),
					})
					continue
				}
			}
			parts = append(parts, part)
		}
		if len(parts) == 0 && len(msg.Content) > 0 {
			continue
		}
		msg.Content = parts
		out = append(out, msg)
	}
	if flattened > 0 || dropped > 0 {
		logger.Debug(ctx, "flattened provider-executed tool history in compaction prompt",
			slog.F("flattened_parts", flattened),
			slog.F("dropped_parts", dropped),
		)
	}
	return out
}

// stringifyToolResultOutput renders a tool result as prompt text. Media
// payloads are summarized so base64 data does not enter the prompt.
func stringifyToolResultOutput(output fantasy.ToolResultOutputContent) string {
	switch typed := output.(type) {
	case fantasy.ToolResultOutputContentText:
		return typed.Text
	case fantasy.ToolResultOutputContentError:
		if typed.Error == nil {
			return "error"
		}
		return typed.Error.Error()
	case fantasy.ToolResultOutputContentMedia:
		if typed.Text != "" {
			return fmt.Sprintf("%s [media %s omitted]", typed.Text, typed.MediaType)
		}
		return fmt.Sprintf("[media %s omitted]", typed.MediaType)
	default:
		return "[unserializable tool output]"
	}
}

// replaceUnsupportedFileParts swaps file parts the compaction model does
// not accept for text placeholders in a copy of messages, so the summary
// notes the attachment existed instead of silently losing it.
func replaceUnsupportedFileParts(
	ctx context.Context,
	logger slog.Logger,
	messages []fantasy.Message,
	acceptsFilePart func(mediaType string) bool,
) []fantasy.Message {
	replaced := 0
	out := make([]fantasy.Message, 0, len(messages))
	for _, msg := range messages {
		parts := make([]fantasy.MessagePart, 0, len(msg.Content))
		for _, part := range msg.Content {
			filePart, ok := part.(fantasy.FilePart)
			if !ok || acceptsFilePart(filePart.MediaType) {
				parts = append(parts, part)
				continue
			}
			replaced++
			parts = append(parts, fantasy.TextPart{
				Text: fmt.Sprintf(
					"[Attachment %q (%s) omitted: not supported by the compaction model]",
					filePart.Filename,
					filePart.MediaType,
				),
				ProviderOptions: filePart.ProviderOptions,
			})
		}
		msg.Content = parts
		out = append(out, msg)
	}
	if replaced > 0 {
		logger.Debug(ctx, "replaced unsupported file parts in compaction prompt",
			slog.F("replaced_parts", replaced),
		)
	}
	return out
}
