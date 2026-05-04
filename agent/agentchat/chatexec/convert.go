//go:build !slim

package chatexec

import (
	"context"
	"encoding/json"
	"strings"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

// MessagesFromSDK converts prompt-ready chat runner wire messages into fantasy
// messages for chatloop execution.
func MessagesFromSDK(
	logger slog.Logger,
	msgs []agentsdk.ChatRunnerMessage,
) ([]fantasy.Message, error) {
	if len(msgs) == 0 {
		return nil, nil
	}

	result := make([]fantasy.Message, 0, len(msgs))
	for i, msg := range msgs {
		role, err := messageRoleFromSDK(msg.Role)
		if err != nil {
			return nil, xerrors.Errorf("convert message %d role: %w", i, err)
		}

		var content []fantasy.MessagePart
		if len(msg.Content) == 0 {
			content = []fantasy.MessagePart{fantasy.TextPart{Text: msg.Text}}
		} else {
			content, err = messagePartsFromSDKParts(logger, msg.Content)
			if err != nil {
				return nil, xerrors.Errorf("convert message %d content: %w", i, err)
			}
		}

		result = append(result, fantasy.Message{
			Role:    role,
			Content: content,
		})
	}
	return result, nil
}

// ContentFromParts converts SDK chat message parts into fantasy content for
// persisted-step reconstruction.
func ContentFromParts(
	logger slog.Logger,
	parts []codersdk.ChatMessagePart,
) ([]fantasy.Content, error) {
	if len(parts) == 0 {
		return nil, nil
	}

	result := make([]fantasy.Content, 0, len(parts))
	for i, part := range parts {
		metadata := providerMetadataFromRaw(logger, part.ProviderMetadata)
		switch part.Type {
		case codersdk.ChatMessagePartTypeText:
			result = append(result, fantasy.TextContent{
				Text:             part.Text,
				ProviderMetadata: metadata,
			})
		case codersdk.ChatMessagePartTypeReasoning:
			result = append(result, fantasy.ReasoningContent{
				Text:             part.Text,
				ProviderMetadata: metadata,
			})
		case codersdk.ChatMessagePartTypeToolCall:
			result = append(result, fantasy.ToolCallContent{
				ToolCallID:       part.ToolCallID,
				ToolName:         part.ToolName,
				Input:            string(part.Args),
				ProviderExecuted: part.ProviderExecuted,
				ProviderMetadata: metadata,
			})
		case codersdk.ChatMessagePartTypeToolResult:
			result = append(result, fantasy.ToolResultContent{
				ToolCallID:       part.ToolCallID,
				ToolName:         part.ToolName,
				Result:           toolResultOutputFromPart(logger, part),
				ProviderExecuted: part.ProviderExecuted,
				ProviderMetadata: metadata,
			})
		default:
			return nil, xerrors.Errorf(
				"unsupported chat message part %q at index %d",
				part.Type,
				i,
			)
		}
	}
	return result, nil
}

// SplitPersistedContent splits persisted fantasy content into assistant parts
// and non-provider-executed tool results for chat runner persist requests.
func SplitPersistedContent(
	content []fantasy.Content,
) (assistantParts, toolResults []codersdk.ChatMessagePart) {
	if len(content) == 0 {
		return nil, nil
	}

	assistantParts = make([]codersdk.ChatMessagePart, 0, len(content))
	for _, block := range content {
		if toolResult, ok := fantasy.AsContentType[fantasy.ToolResultContent](block); ok && !toolResult.ProviderExecuted {
			toolResults = append(toolResults, chatprompt.PartFromContent(toolResult))
			continue
		}
		assistantParts = append(assistantParts, chatprompt.PartFromContent(block))
	}

	if len(assistantParts) == 0 {
		assistantParts = nil
	}
	if len(toolResults) == 0 {
		toolResults = nil
	}
	return assistantParts, toolResults
}

// UsageToSDK converts fantasy usage metrics into the chat runner wire format.
// It returns nil for a zero-value usage to preserve the optional request field.
func UsageToSDK(u fantasy.Usage) *agentsdk.ChatRunnerUsage {
	if u == (fantasy.Usage{}) {
		return nil
	}

	return &agentsdk.ChatRunnerUsage{
		InputTokens:         u.InputTokens,
		OutputTokens:        u.OutputTokens,
		TotalTokens:         u.TotalTokens,
		ReasoningTokens:     u.ReasoningTokens,
		CacheCreationTokens: u.CacheCreationTokens,
		CacheReadTokens:     u.CacheReadTokens,
	}
}

func messageRoleFromSDK(role string) (fantasy.MessageRole, error) {
	switch codersdk.ChatMessageRole(role) {
	case codersdk.ChatMessageRoleSystem:
		return fantasy.MessageRoleSystem, nil
	case codersdk.ChatMessageRoleUser:
		return fantasy.MessageRoleUser, nil
	case codersdk.ChatMessageRoleAssistant:
		return fantasy.MessageRoleAssistant, nil
	case codersdk.ChatMessageRoleTool:
		return fantasy.MessageRoleTool, nil
	default:
		return "", xerrors.Errorf("unsupported chat runner message role %q", role)
	}
}

func messagePartsFromSDKParts(
	logger slog.Logger,
	parts []codersdk.ChatMessagePart,
) ([]fantasy.MessagePart, error) {
	if len(parts) == 0 {
		return nil, nil
	}

	result := make([]fantasy.MessagePart, 0, len(parts))
	for i, part := range parts {
		opts := providerOptionsFromRaw(logger, part.ProviderMetadata)
		switch part.Type {
		case codersdk.ChatMessagePartTypeText:
			result = append(result, fantasy.TextPart{
				Text:            part.Text,
				ProviderOptions: opts,
			})
		case codersdk.ChatMessagePartTypeReasoning:
			result = append(result, fantasy.ReasoningPart{
				Text:            part.Text,
				ProviderOptions: opts,
			})
		case codersdk.ChatMessagePartTypeToolCall:
			result = append(result, fantasy.ToolCallPart{
				ToolCallID:       part.ToolCallID,
				ToolName:         part.ToolName,
				Input:            string(part.Args),
				ProviderExecuted: part.ProviderExecuted,
				ProviderOptions:  opts,
			})
		case codersdk.ChatMessagePartTypeToolResult:
			result = append(result, fantasy.ToolResultPart{
				ToolCallID:       part.ToolCallID,
				Output:           toolResultOutputFromPart(logger, part),
				ProviderExecuted: part.ProviderExecuted,
				ProviderOptions:  opts,
			})
		default:
			return nil, xerrors.Errorf(
				"unsupported chat message part %q at index %d",
				part.Type,
				i,
			)
		}
	}
	return result, nil
}

func providerMetadataFromRaw(
	logger slog.Logger,
	raw json.RawMessage,
) fantasy.ProviderMetadata {
	if len(raw) == 0 {
		return nil
	}

	var intermediate map[string]json.RawMessage
	if err := json.Unmarshal(raw, &intermediate); err != nil {
		logger.Warn(
			context.Background(),
			"failed to unmarshal provider metadata",
			slog.Error(err),
		)
		return nil
	}
	metadata, err := fantasy.UnmarshalProviderMetadata(intermediate)
	if err != nil {
		logger.Warn(
			context.Background(),
			"failed to decode provider metadata",
			slog.Error(err),
		)
		return nil
	}
	return metadata
}

func providerOptionsFromRaw(
	logger slog.Logger,
	raw json.RawMessage,
) fantasy.ProviderOptions {
	metadata := providerMetadataFromRaw(logger, raw)
	if len(metadata) == 0 {
		return nil
	}
	return fantasy.ProviderOptions(metadata)
}

func toolResultOutputFromPart(
	logger slog.Logger,
	part codersdk.ChatMessagePart,
) fantasy.ToolResultOutputContent {
	resultText := string(part.Result)
	if resultText == "" || resultText == "null" {
		resultText = "{}"
	}

	if part.IsError {
		message := strings.TrimSpace(resultText)
		if extracted := extractErrorString(part.Result); extracted != "" {
			message = extracted
		}
		return fantasy.ToolResultOutputContentError{
			Error: xerrors.New(message),
		}
	}

	if part.IsMedia {
		var media persistedMediaResult
		unmarshalErr := json.Unmarshal(part.Result, &media)
		if unmarshalErr == nil && media.Data != "" && media.MimeType != "" {
			return fantasy.ToolResultOutputContentMedia{
				Data:      media.Data,
				MediaType: media.MimeType,
				Text:      media.Text,
			}
		}

		fields := []slog.Field{
			slog.F("tool_call_id", part.ToolCallID),
			slog.F("tool_name", part.ToolName),
			slog.F("has_data", media.Data != ""),
			slog.F("has_mime_type", media.MimeType != ""),
		}
		if unmarshalErr != nil {
			fields = append(fields, slog.Error(unmarshalErr))
		}
		logger.Warn(
			context.Background(),
			"media tool result failed reconstruction, falling through to text",
			fields...,
		)
	}

	return fantasy.ToolResultOutputContentText{Text: resultText}
}

func extractErrorString(raw json.RawMessage) string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ""
	}
	errField, ok := fields["error"]
	if !ok {
		return ""
	}
	var message string
	if err := json.Unmarshal(errField, &message); err != nil {
		return ""
	}
	return strings.TrimSpace(message)
}

type persistedMediaResult struct {
	Data     string `json:"data"`
	MimeType string `json:"mime_type"`
	Text     string `json:"text"`
}
