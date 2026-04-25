package chatprompt

import (
	"context"

	"charm.land/fantasy"

	"cdr.dev/slog/v3"
)

// OpenAIResponsesSanitizationStats describes prompt-history changes made
// for OpenAI Responses compatibility.
type OpenAIResponsesSanitizationStats struct {
	RemovedToolCalls        int
	RemovedToolResults      int
	RemovedWebSearchCalls   int
	RemovedWebSearchResults int
	DroppedMessages         int
	DisabledChainMode       bool
	UnsafeForChainMode      bool
}

// AnalyzeOpenAIResponsesMessages reports whether prompt history contains
// tool-call shapes that are unsafe for OpenAI Responses chaining.
func AnalyzeOpenAIResponsesMessages(messages []fantasy.Message) OpenAIResponsesSanitizationStats {
	classification := classifyOpenAIResponsesMessages(messages)
	return classification.stats
}

// SanitizeOpenAIResponsesMessages removes invalid OpenAI Responses tool-call
// history while preserving valid content and same-role coalescing behavior.
func SanitizeOpenAIResponsesMessages(messages []fantasy.Message) ([]fantasy.Message, OpenAIResponsesSanitizationStats) {
	classification := classifyOpenAIResponsesMessages(messages)
	if len(classification.remove) == 0 {
		return messages, OpenAIResponsesSanitizationStats{}
	}

	stats := classification.stats
	out := make([]fantasy.Message, 0, len(messages))
	for messageIndex, msg := range messages {
		changedMessage := false
		var parts []fantasy.MessagePart
		for partIndex, part := range msg.Content {
			key := openAIResponsesPartKey{
				messageIndex: messageIndex,
				partIndex:    partIndex,
			}
			if _, remove := classification.remove[key]; remove {
				if !changedMessage {
					parts = make([]fantasy.MessagePart, 0, len(msg.Content)-1)
					parts = append(parts, msg.Content[:partIndex]...)
					changedMessage = true
				}
				continue
			}
			if changedMessage {
				parts = append(parts, part)
			}
		}

		if changedMessage {
			if len(parts) == 0 {
				stats.DroppedMessages++
				continue
			}
			msg.Content = parts
		}
		out = appendSanitizedMessage(out, msg)
	}
	return out, stats
}

// LogOpenAIResponsesSanitization logs counts for prompt-history sanitization.
func LogOpenAIResponsesSanitization(
	ctx context.Context,
	logger slog.Logger,
	phase string,
	provider string,
	modelName string,
	stats OpenAIResponsesSanitizationStats,
	extra ...slog.Field,
) {
	hasRemoval := stats.RemovedToolCalls > 0 ||
		stats.RemovedToolResults > 0 ||
		stats.RemovedWebSearchCalls > 0 ||
		stats.RemovedWebSearchResults > 0
	if !hasRemoval && !stats.DisabledChainMode {
		return
	}

	fields := []slog.Field{
		slog.F("phase", phase),
		slog.F("provider", provider),
		slog.F("model", modelName),
		slog.F("removed_tool_calls", stats.RemovedToolCalls),
		slog.F("removed_tool_results", stats.RemovedToolResults),
		slog.F("removed_web_search_calls", stats.RemovedWebSearchCalls),
		slog.F("removed_web_search_results", stats.RemovedWebSearchResults),
		slog.F("dropped_messages", stats.DroppedMessages),
		slog.F("disabled_chain_mode", stats.DisabledChainMode),
		slog.F("unsafe_for_chain_mode", stats.UnsafeForChainMode),
	}
	fields = append(fields, extra...)
	logger.Warn(ctx, "sanitized OpenAI Responses prompt history", fields...)
}

type openAIResponsesPartKey struct {
	messageIndex int
	partIndex    int
}

type openAIResponsesOccurrence struct {
	key       openAIResponsesPartKey
	order     int
	webSearch bool
}

type openAIResponsesToolHistory struct {
	calls     []openAIResponsesOccurrence
	results   []openAIResponsesOccurrence
	webSearch bool
}

type openAIResponsesRemoval struct {
	call      bool
	webSearch bool
}

type openAIResponsesClassification struct {
	remove map[openAIResponsesPartKey]openAIResponsesRemoval
	stats  OpenAIResponsesSanitizationStats
}

func classifyOpenAIResponsesMessages(messages []fantasy.Message) openAIResponsesClassification {
	classification := openAIResponsesClassification{}
	histories := make(map[string]*openAIResponsesToolHistory)

	order := 0
	for messageIndex, msg := range messages {
		for partIndex, part := range msg.Content {
			key := openAIResponsesPartKey{
				messageIndex: messageIndex,
				partIndex:    partIndex,
			}
			if toolCall, ok := fantasy.AsMessagePart[fantasy.ToolCallPart](part); ok {
				webSearch := isOpenAIResponsesWebSearchCall(toolCall)
				if toolCall.ToolCallID == "" {
					classification.removeToolCall(openAIResponsesOccurrence{key: key})
					order++
					continue
				}
				history := openAIResponsesHistoryForID(histories, toolCall.ToolCallID)
				history.calls = append(history.calls, openAIResponsesOccurrence{
					key:       key,
					order:     order,
					webSearch: webSearch,
				})
				if webSearch {
					history.webSearch = true
				}
			} else if toolResult, ok := fantasy.AsMessagePart[fantasy.ToolResultPart](part); ok {
				if toolResult.ToolCallID == "" {
					classification.removeToolResult(openAIResponsesOccurrence{key: key})
					order++
					continue
				}
				history := openAIResponsesHistoryForID(histories, toolResult.ToolCallID)
				history.results = append(history.results, openAIResponsesOccurrence{
					key:   key,
					order: order,
				})
			}
			order++
		}
	}

	for _, history := range histories {
		invalid := history.webSearch || len(history.calls) != 1 || len(history.results) != 1
		if !invalid && history.results[0].order < history.calls[0].order {
			invalid = true
		}
		if !invalid {
			continue
		}

		for _, call := range history.calls {
			classification.removeToolCall(call)
		}
		for _, result := range history.results {
			result.webSearch = history.webSearch
			classification.removeToolResult(result)
		}
	}

	return classification
}

func openAIResponsesHistoryForID(
	histories map[string]*openAIResponsesToolHistory,
	toolCallID string,
) *openAIResponsesToolHistory {
	history, ok := histories[toolCallID]
	if ok {
		return history
	}
	history = &openAIResponsesToolHistory{}
	histories[toolCallID] = history
	return history
}

func (c *openAIResponsesClassification) removeToolCall(occ openAIResponsesOccurrence) {
	if c.remove == nil {
		c.remove = make(map[openAIResponsesPartKey]openAIResponsesRemoval)
	}
	if _, ok := c.remove[occ.key]; ok {
		return
	}
	c.remove[occ.key] = openAIResponsesRemoval{call: true, webSearch: occ.webSearch}
	c.stats.RemovedToolCalls++
	if occ.webSearch {
		c.stats.RemovedWebSearchCalls++
	}
	c.stats.UnsafeForChainMode = true
}

func (c *openAIResponsesClassification) removeToolResult(occ openAIResponsesOccurrence) {
	if c.remove == nil {
		c.remove = make(map[openAIResponsesPartKey]openAIResponsesRemoval)
	}
	if _, ok := c.remove[occ.key]; ok {
		return
	}
	c.remove[occ.key] = openAIResponsesRemoval{call: false, webSearch: occ.webSearch}
	c.stats.RemovedToolResults++
	if occ.webSearch {
		c.stats.RemovedWebSearchResults++
	}
	c.stats.UnsafeForChainMode = true
}

func isOpenAIResponsesWebSearchCall(toolCall fantasy.ToolCallPart) bool {
	if !toolCall.ProviderExecuted {
		return false
	}
	return toolCall.ToolName == "web_search" || toolCall.ToolName == "web_search_preview"
}
