package chatsanitize

import (
	"context"
	"encoding/json"
	"strings"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"

	"cdr.dev/slog/v3"
)

const maxAnthropicProviderToolViolationLogDetails = 32

// supportedAnthropicProviderToolNames is the allowlist of provider-executed
// tool names the Anthropic provider in fantasy can currently serialize.
var supportedAnthropicProviderToolNames = map[string]struct{}{
	"web_search": {},
}

const (
	anthropicProviderToolViolationOutsideAssistant = "provider_executed_block_outside_assistant"
	anthropicProviderToolViolationOrphanCall       = "provider_executed_call_without_result"
	anthropicProviderToolViolationOrphanResult     = "provider_executed_result_without_call"
	anthropicProviderToolViolationDuplicateID      = "duplicate_provider_executed_id"
	anthropicProviderToolViolationResultBeforeCall = "provider_executed_result_before_call"
	anthropicProviderToolViolationInvalidCall      = "invalid_provider_executed_tool_call"
	anthropicProviderToolViolationInvalidResult    = "invalid_provider_executed_tool_result"
)

// AnthropicProviderToolSanitizationStats describes prompt changes made
// while removing invalid Anthropic provider-executed tool history.
type AnthropicProviderToolSanitizationStats struct {
	RemovedToolCalls   int
	RemovedToolResults int
	DroppedMessages    int
}

// AnthropicProviderToolHistoryViolation describes an invalid
// provider-executed tool history block in an Anthropic prompt.
type AnthropicProviderToolHistoryViolation struct {
	MessageIndex int
	PartIndex    int
	ID           string
	Reason       string
}

// LogAnthropicProviderToolSanitization logs prompt changes made while
// removing invalid Anthropic provider-executed tool history.
func LogAnthropicProviderToolSanitization(
	ctx context.Context,
	logger slog.Logger,
	phase string,
	provider string,
	modelName string,
	stats AnthropicProviderToolSanitizationStats,
	extra ...slog.Field,
) {
	if stats.RemovedToolCalls == 0 && stats.RemovedToolResults == 0 {
		return
	}
	fields := []slog.Field{
		slog.F("phase", phase),
		slog.F("tool_type", "provider_executed"),
		slog.F("provider", provider),
		slog.F("model", modelName),
		slog.F("removed_tool_calls", stats.RemovedToolCalls),
		slog.F("removed_tool_results", stats.RemovedToolResults),
		slog.F("dropped_messages", stats.DroppedMessages),
	}
	fields = append(fields, extra...)
	logger.Warn(ctx, "removed provider-executed tool history", fields...)
}

// IsSerializableAnthropicProviderToolCall reports whether part can be
// serialized as an Anthropic provider-executed tool call.
func IsSerializableAnthropicProviderToolCall(part fantasy.MessagePart) bool {
	toolCall, ok := safeMessageToolCallPart(part)
	if !ok || !toolCall.ProviderExecuted {
		return false
	}
	if strings.TrimSpace(toolCall.ToolCallID) == "" || toolCall.ToolName == "" {
		return false
	}
	if !IsAllowedAnthropicProviderToolName(toolCall.ToolName) {
		return false
	}
	return json.Valid([]byte(strings.TrimSpace(toolCall.Input)))
}

// IsSerializableAnthropicProviderToolResult reports whether part can be
// serialized as an Anthropic provider-executed tool result for matchedCall.
func IsSerializableAnthropicProviderToolResult(
	part fantasy.MessagePart,
	matchedCall fantasy.MessagePart,
) bool {
	result, ok := safeMessageToolResultPart(part)
	if !ok || !result.ProviderExecuted {
		return false
	}
	if strings.TrimSpace(result.ToolCallID) == "" {
		return false
	}
	toolCall, ok := safeMessageToolCallPart(matchedCall)
	if !ok || result.ToolCallID != toolCall.ToolCallID {
		return false
	}
	if !IsSerializableAnthropicProviderToolCall(matchedCall) {
		return false
	}
	return hasSerializableAnthropicProviderToolResultMetadata(result, toolCall)
}

func hasSerializableAnthropicProviderToolResultMetadata(
	result fantasy.ToolResultPart,
	matchedCall fantasy.ToolCallPart,
) bool {
	if matchedCall.ToolName != "web_search" {
		return false
	}
	providerMetadata := result.ProviderOptions[fantasyanthropic.Name]
	metadata, ok := providerMetadata.(*fantasyanthropic.WebSearchResultMetadata)
	return ok && metadata != nil
}

// AnthropicProviderToolResultTextPart converts a provider-executed tool
// result into text so unsafe provider-tool structure can be removed without
// losing the result payload.
func AnthropicProviderToolResultTextPart(
	part fantasy.MessagePart,
) (fantasy.TextPart, bool) {
	var zero fantasy.TextPart
	result, ok := safeMessageToolResultPart(part)
	if !ok || !result.ProviderExecuted {
		return zero, false
	}
	text := AnthropicToolResultOutputText(result.Output)
	if text == "" {
		return zero, false
	}
	return fantasy.TextPart{Text: text}, true
}

// AnthropicToolResultOutputText converts a tool result payload into the text
// that should remain in the prompt when provider-tool metadata is unsafe.
func AnthropicToolResultOutputText(output fantasy.ToolResultOutputContent) string {
	switch value := output.(type) {
	case fantasy.ToolResultOutputContentText:
		return value.Text
	case *fantasy.ToolResultOutputContentText:
		if value == nil {
			return ""
		}
		return value.Text
	case fantasy.ToolResultOutputContentError:
		if value.Error == nil {
			return ""
		}
		return value.Error.Error()
	case *fantasy.ToolResultOutputContentError:
		if value == nil || value.Error == nil {
			return ""
		}
		return value.Error.Error()
	case fantasy.ToolResultOutputContentMedia:
		return value.Text
	case *fantasy.ToolResultOutputContentMedia:
		if value == nil {
			return ""
		}
		return value.Text
	}

	if output == nil {
		return ""
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// IsAllowedAnthropicProviderToolName reports whether name is an Anthropic
// provider-executed tool name we know how to serialize.
func IsAllowedAnthropicProviderToolName(name string) bool {
	_, ok := supportedAnthropicProviderToolNames[name]
	return ok
}

// ValidateAnthropicProviderToolHistory returns violations found in messages
// with invalid Anthropic provider-executed tool history blocks.
func ValidateAnthropicProviderToolHistory(
	messages []fantasy.Message,
) []AnthropicProviderToolHistoryViolation {
	analysis := analyzeAnthropicProviderToolHistory(messages)
	return analysis.violations
}

// AnthropicProviderToolPartsToRemove returns provider-executed tool parts
// that cannot be serialized safely in a single Anthropic assistant message.
// Violation MessageIndex values refer to the synthetic assistant message, so
// they are always 0.
func AnthropicProviderToolPartsToRemove(
	provider string,
	parts []fantasy.MessagePart,
) (map[int]struct{}, []AnthropicProviderToolHistoryViolation) {
	remove := make(map[int]struct{})
	if provider != fantasyanthropic.Name || len(parts) == 0 {
		return remove, nil
	}

	analysis := analyzeAnthropicProviderToolHistory([]fantasy.Message{{
		Role:    fantasy.MessageRoleAssistant,
		Content: parts,
	}})
	for key := range analysis.remove {
		if key.messageIndex != 0 {
			continue
		}
		remove[key.partIndex] = struct{}{}
	}

	violations := make([]AnthropicProviderToolHistoryViolation, len(analysis.violations))
	copy(violations, analysis.violations)
	return remove, violations
}

// SanitizeAnthropicProviderToolHistory removes Anthropic provider-executed
// tool history that cannot be serialized safely.
func SanitizeAnthropicProviderToolHistory(
	provider string,
	messages []fantasy.Message,
) ([]fantasy.Message, AnthropicProviderToolSanitizationStats) {
	var stats AnthropicProviderToolSanitizationStats
	if provider != fantasyanthropic.Name || len(messages) == 0 {
		return messages, stats
	}

	current := messages
	changed := false
	for {
		// Each pass shrinks the finite part set, so the loop terminates.
		analysis := analyzeAnthropicProviderToolHistory(current)
		if len(analysis.remove) == 0 {
			if !changed {
				return messages, stats
			}
			return current, stats
		}

		out := make([]fantasy.Message, 0, len(current))
		for messageIndex, msg := range current {
			parts := make([]fantasy.MessagePart, 0, len(msg.Content))
			removedFromMessage := 0
			for partIndex, part := range msg.Content {
				key := anthropicProviderToolPartKey{
					messageIndex: messageIndex,
					partIndex:    partIndex,
				}
				if _, remove := analysis.remove[key]; remove {
					countRemovedAnthropicProviderToolPart(&stats, part)
					if textPart, ok := AnthropicProviderToolResultTextPart(part); ok {
						parts = append(parts, textPart)
					}
					removedFromMessage++
					changed = true
					continue
				}
				parts = append(parts, part)
			}

			if removedFromMessage > 0 {
				if len(parts) == 0 {
					stats.DroppedMessages++
					continue
				}
				msg.Content = parts
			}
			out = appendSanitizedMessage(out, msg)
		}
		current = out
	}
}

// SanitizeAnthropicProviderToolStepContent removes invalid Anthropic
// provider-executed tool content from a streamed step and logs removals.
func SanitizeAnthropicProviderToolStepContent(
	ctx context.Context,
	logger slog.Logger,
	provider string,
	modelName string,
	phase string,
	step int,
	finishReason fantasy.FinishReason,
	content []fantasy.Content,
) []fantasy.Content {
	sanitized, stats := SanitizeAnthropicProviderToolContent(provider, content)
	LogAnthropicProviderToolSanitization(
		ctx, logger, phase, provider, modelName, stats,
		slog.F("step_index", step),
		slog.F("finish_reason", finishReason),
	)
	return sanitized
}

// SanitizeAnthropicProviderToolContent removes invalid Anthropic
// provider-executed tool blocks from streamed content.
func SanitizeAnthropicProviderToolContent(
	provider string,
	content []fantasy.Content,
) ([]fantasy.Content, AnthropicProviderToolSanitizationStats) {
	var stats AnthropicProviderToolSanitizationStats
	if provider != fantasyanthropic.Name || len(content) == 0 {
		return content, stats
	}

	partIndexByContentIndex := make([]int, len(content))
	for index := range partIndexByContentIndex {
		partIndexByContentIndex[index] = noMappedToolPartIndex
	}
	contentKinds := make([]mappedToolContentKind, len(content))
	parts := make([]fantasy.MessagePart, 0, len(content))
	providerCalls := make(map[string][]mappedProviderToolCall)
	providerResultNames := make(map[string][]string)
	for contentIndex, block := range content {
		if toolCall, ok := safeToolCallContent(block); ok {
			partIndex := len(parts)
			parts = append(parts, toolCallContentToPart(toolCall))
			partIndexByContentIndex[contentIndex] = partIndex
			contentKinds[contentIndex] = mappedToolContentCall
			if toolCall.ProviderExecuted {
				providerCalls[toolCall.ToolCallID] = append(
					providerCalls[toolCall.ToolCallID],
					mappedProviderToolCall{
						partIndex: partIndex,
						toolName:  toolCall.ToolName,
					},
				)
			}
			continue
		}
		if toolResult, ok := safeToolResultContent(block); ok {
			partIndex := len(parts)
			parts = append(parts, toolResultContentToPart(toolResult))
			partIndexByContentIndex[contentIndex] = partIndex
			contentKinds[contentIndex] = mappedToolContentResult
			if toolResult.ProviderExecuted {
				providerResultNames[toolResult.ToolCallID] = append(
					providerResultNames[toolResult.ToolCallID],
					toolResult.ToolName,
				)
			}
		}
	}
	if len(parts) == 0 {
		return content, stats
	}

	// ToolResultContent carries ToolName, but ToolResultPart does not. Preserve
	// the content sanitizer mismatch check by invalidating the synthetic call.
	for id, calls := range providerCalls {
		for _, call := range calls {
			for _, resultToolName := range providerResultNames[id] {
				if resultToolName == "" || resultToolName == call.toolName {
					continue
				}
				toolCall, ok := parts[call.partIndex].(fantasy.ToolCallPart)
				if !ok {
					break
				}
				toolCall.ToolName = ""
				parts[call.partIndex] = toolCall
				break
			}
		}
	}

	removeParts, _ := AnthropicProviderToolPartsToRemove(provider, parts)
	if len(removeParts) == 0 {
		return content, stats
	}

	removeContent := make(map[int]struct{}, len(removeParts))
	for contentIndex, partIndex := range partIndexByContentIndex {
		if partIndex == noMappedToolPartIndex {
			continue
		}
		if _, remove := removeParts[partIndex]; remove {
			removeContent[contentIndex] = struct{}{}
		}
	}
	if len(removeContent) == 0 {
		return content, stats
	}

	out := make([]fantasy.Content, 0, len(content))
	for contentIndex, block := range content {
		if _, remove := removeContent[contentIndex]; remove {
			switch contentKinds[contentIndex] {
			case mappedToolContentCall:
				stats.RemovedToolCalls++
			case mappedToolContentResult:
				stats.RemovedToolResults++
				if textContent, ok := anthropicProviderToolResultTextContent(block); ok {
					out = append(out, textContent)
				}
			}
			continue
		}
		out = append(out, block)
	}
	return out, stats
}

// IsAnthropicProviderExecutedToolCall reports whether toolCall is an
// Anthropic provider-executed tool call.
func IsAnthropicProviderExecutedToolCall(
	provider string,
	toolCall fantasy.ToolCallContent,
) bool {
	return provider == fantasyanthropic.Name && toolCall.ProviderExecuted
}

// ApplyAnthropicProviderToolGuard fail-closes unsafe Anthropic provider-tool
// history immediately before a provider request is issued.
func ApplyAnthropicProviderToolGuard(
	ctx context.Context,
	logger slog.Logger,
	provider string,
	modelName string,
	messages []fantasy.Message,
) []fantasy.Message {
	if provider != fantasyanthropic.Name || len(messages) == 0 {
		return messages
	}

	violations := ValidateAnthropicProviderToolHistory(messages)
	if len(violations) == 0 {
		return messages
	}
	affectedMessages := messageIndexesFromAnthropicProviderToolViolations(
		violations,
		len(messages),
	)
	guarded := sanitizeAnthropicProviderToolGuardMessages(
		ctx,
		logger,
		provider,
		modelName,
		messages,
		affectedMessages,
		len(violations),
	)
	if isSafeAnthropicProviderToolPrompt(guarded) {
		return guarded
	}

	fallbackViolations := ValidateAnthropicProviderToolHistory(guarded)
	fallbackAffectedMessages := providerExecutedToolMessageIndexes(guarded)
	guarded = sanitizeAnthropicProviderToolGuardMessages(
		ctx,
		logger,
		provider,
		modelName,
		guarded,
		fallbackAffectedMessages,
		len(fallbackViolations),
		slog.F("fallback", true),
	)
	if isSafeAnthropicProviderToolPrompt(guarded) {
		return guarded
	}

	// The guard sanitizer should normally remove every typed provider block it
	// selects. The strip path is a fail-closed backstop for analyzer and
	// provider serialization drift, not a path we can drive without hooks.
	preStripViolations := ValidateAnthropicProviderToolHistory(guarded)
	stripMessages := messageIndexesFromAnthropicProviderToolViolations(
		preStripViolations,
		len(guarded),
	)

	var stripStats AnthropicProviderToolSanitizationStats
	guarded, stripStats = stripAnthropicProviderToolHistoryFromMessages(
		guarded,
		stripMessages,
	)
	var sanitizeStats AnthropicProviderToolSanitizationStats
	guarded, sanitizeStats = SanitizeAnthropicProviderToolHistory(
		provider,
		guarded,
	)
	stripStats = addAnthropicProviderToolSanitizationStats(stripStats, sanitizeStats)

	if !isSafeAnthropicProviderToolPrompt(guarded) {
		guarded, sanitizeStats = stripAnthropicProviderToolHistoryFromMessages(
			guarded,
			providerExecutedToolMessageIndexes(guarded),
		)
		stripStats = addAnthropicProviderToolSanitizationStats(stripStats, sanitizeStats)
		guarded, sanitizeStats = SanitizeAnthropicProviderToolHistory(
			provider,
			guarded,
		)
		stripStats = addAnthropicProviderToolSanitizationStats(stripStats, sanitizeStats)
		if !isSafeAnthropicProviderToolPrompt(guarded) {
			logger.Error(
				ctx,
				"anthropic provider tool guard postcondition failed: prompt still unsafe after nuclear strip",
				slog.F("phase", "pre_request_guard_postcondition_failed"),
				slog.F("tool_type", "provider_executed"),
				slog.F("provider", provider),
				slog.F("model", modelName),
			)
		}
	}

	details, truncated := anthropicProviderToolViolationLogDetails(
		preStripViolations,
	)
	LogAnthropicProviderToolSanitization(
		ctx,
		logger,
		"pre_request_guard_fallback_strip",
		provider,
		modelName,
		stripStats,
		slog.F("validation_violations", len(preStripViolations)),
		slog.F("validation_violation_details", details),
		slog.F("truncated_violations", truncated),
	)
	return guarded
}

type anthropicProviderToolPartKey struct {
	messageIndex int
	partIndex    int
}

type anthropicProviderToolHistoryAnalysis struct {
	remove     map[anthropicProviderToolPartKey]struct{}
	violations []AnthropicProviderToolHistoryViolation
}

type anthropicProviderToolOccurrence struct {
	partIndex int
	part      fantasy.MessagePart
}

type anthropicProviderToolIDHistory struct {
	calls   []anthropicProviderToolOccurrence
	results []anthropicProviderToolOccurrence
}

func analyzeAnthropicProviderToolHistory(
	messages []fantasy.Message,
) anthropicProviderToolHistoryAnalysis {
	analysis := anthropicProviderToolHistoryAnalysis{
		remove: make(map[anthropicProviderToolPartKey]struct{}),
	}
	for messageIndex, msg := range messages {
		if msg.Role != fantasy.MessageRoleAssistant {
			for partIndex, part := range msg.Content {
				id, ok := anthropicProviderExecutedToolPartID(part)
				if !ok {
					continue
				}
				analysis.addViolation(
					messageIndex,
					partIndex,
					id,
					anthropicProviderToolViolationOutsideAssistant,
				)
			}
			continue
		}
		analysis.analyzeAssistantMessage(messageIndex, msg)
	}
	return analysis
}

func (a *anthropicProviderToolHistoryAnalysis) analyzeAssistantMessage(
	messageIndex int,
	msg fantasy.Message,
) {
	histories := make(map[string]*anthropicProviderToolIDHistory)
	ids := make([]string, 0)
	for partIndex, part := range msg.Content {
		if toolCall, ok := safeMessageToolCallPart(part); ok && toolCall.ProviderExecuted {
			history := ensureAnthropicProviderToolIDHistory(
				histories,
				&ids,
				toolCall.ToolCallID,
			)
			history.calls = append(history.calls, anthropicProviderToolOccurrence{
				partIndex: partIndex,
				part:      part,
			})
			continue
		}
		if result, ok := safeMessageToolResultPart(part); ok && result.ProviderExecuted {
			history := ensureAnthropicProviderToolIDHistory(
				histories,
				&ids,
				result.ToolCallID,
			)
			history.results = append(history.results, anthropicProviderToolOccurrence{
				partIndex: partIndex,
				part:      part,
			})
		}
	}

	for _, id := range ids {
		history := histories[id]
		switch {
		case len(history.calls) > 1 || len(history.results) > 1:
			a.addHistoryViolations(
				messageIndex,
				id,
				history,
				anthropicProviderToolViolationDuplicateID,
			)
		case len(history.calls) == 1 && len(history.results) == 0:
			a.addOccurrenceViolation(
				messageIndex,
				id,
				history.calls[0],
				anthropicProviderToolViolationOrphanCall,
			)
		case len(history.calls) == 0 && len(history.results) == 1:
			a.addOccurrenceViolation(
				messageIndex,
				id,
				history.results[0],
				anthropicProviderToolViolationOrphanResult,
			)
		case len(history.calls) == 1 && len(history.results) == 1:
			call := history.calls[0]
			result := history.results[0]
			if call.partIndex >= result.partIndex {
				a.addHistoryViolations(
					messageIndex,
					id,
					history,
					anthropicProviderToolViolationResultBeforeCall,
				)
				continue
			}
			if !IsSerializableAnthropicProviderToolCall(call.part) {
				a.addHistoryViolations(
					messageIndex,
					id,
					history,
					anthropicProviderToolViolationInvalidCall,
				)
				continue
			}
			if !IsSerializableAnthropicProviderToolResult(result.part, call.part) {
				a.addHistoryViolations(
					messageIndex,
					id,
					history,
					anthropicProviderToolViolationInvalidResult,
				)
			}
		}
	}
}

func ensureAnthropicProviderToolIDHistory(
	histories map[string]*anthropicProviderToolIDHistory,
	ids *[]string,
	id string,
) *anthropicProviderToolIDHistory {
	history, ok := histories[id]
	if ok {
		return history
	}
	history = &anthropicProviderToolIDHistory{}
	histories[id] = history
	*ids = append(*ids, id)
	return history
}

func (a *anthropicProviderToolHistoryAnalysis) addHistoryViolations(
	messageIndex int,
	id string,
	history *anthropicProviderToolIDHistory,
	reason string,
) {
	for _, occurrence := range history.calls {
		a.addOccurrenceViolation(messageIndex, id, occurrence, reason)
	}
	for _, occurrence := range history.results {
		a.addOccurrenceViolation(messageIndex, id, occurrence, reason)
	}
}

func (a *anthropicProviderToolHistoryAnalysis) addOccurrenceViolation(
	messageIndex int,
	id string,
	occurrence anthropicProviderToolOccurrence,
	reason string,
) {
	a.addViolation(messageIndex, occurrence.partIndex, id, reason)
}

func (a *anthropicProviderToolHistoryAnalysis) addViolation(
	messageIndex int,
	partIndex int,
	id string,
	reason string,
) {
	key := anthropicProviderToolPartKey{
		messageIndex: messageIndex,
		partIndex:    partIndex,
	}
	if _, ok := a.remove[key]; ok {
		return
	}
	a.remove[key] = struct{}{}
	a.violations = append(a.violations, AnthropicProviderToolHistoryViolation{
		MessageIndex: messageIndex,
		PartIndex:    partIndex,
		ID:           id,
		Reason:       reason,
	})
}

func anthropicProviderExecutedToolPartID(part fantasy.MessagePart) (string, bool) {
	if toolCall, ok := safeMessageToolCallPart(part); ok && toolCall.ProviderExecuted {
		return toolCall.ToolCallID, true
	}
	if result, ok := safeMessageToolResultPart(part); ok && result.ProviderExecuted {
		return result.ToolCallID, true
	}
	return "", false
}

func countRemovedAnthropicProviderToolPart(
	stats *AnthropicProviderToolSanitizationStats,
	part fantasy.MessagePart,
) {
	if toolCall, ok := safeMessageToolCallPart(part); ok && toolCall.ProviderExecuted {
		stats.RemovedToolCalls++
		return
	}
	if result, ok := safeMessageToolResultPart(part); ok && result.ProviderExecuted {
		stats.RemovedToolResults++
	}
}

const noMappedToolPartIndex = -1

type mappedToolContentKind int

const (
	_ mappedToolContentKind = iota
	mappedToolContentCall
	mappedToolContentResult
)

type mappedProviderToolCall struct {
	partIndex int
	toolName  string
}

func anthropicProviderToolResultTextContent(
	block fantasy.Content,
) (fantasy.TextContent, bool) {
	var zero fantasy.TextContent
	toolResult, ok := safeToolResultContent(block)
	if !ok || !toolResult.ProviderExecuted {
		return zero, false
	}
	text := AnthropicToolResultOutputText(toolResult.Result)
	if text == "" {
		return zero, false
	}
	return fantasy.TextContent{Text: text}, true
}

func safeToolCallContent(block fantasy.Content) (fantasy.ToolCallContent, bool) {
	var zero fantasy.ToolCallContent
	switch value := block.(type) {
	case fantasy.ToolCallContent:
		return value, true
	case *fantasy.ToolCallContent:
		if value == nil {
			return zero, false
		}
		return *value, true
	default:
		return zero, false
	}
}

func safeToolResultContent(block fantasy.Content) (fantasy.ToolResultContent, bool) {
	var zero fantasy.ToolResultContent
	switch value := block.(type) {
	case fantasy.ToolResultContent:
		return value, true
	case *fantasy.ToolResultContent:
		if value == nil {
			return zero, false
		}
		return *value, true
	default:
		return zero, false
	}
}

func toolCallContentToPart(toolCall fantasy.ToolCallContent) fantasy.ToolCallPart {
	return fantasy.ToolCallPart{
		ToolCallID:       toolCall.ToolCallID,
		ToolName:         toolCall.ToolName,
		Input:            toolCall.Input,
		ProviderExecuted: toolCall.ProviderExecuted,
		ProviderOptions:  fantasy.ProviderOptions(toolCall.ProviderMetadata),
	}
}

func toolResultContentToPart(toolResult fantasy.ToolResultContent) fantasy.ToolResultPart {
	return fantasy.ToolResultPart{
		ToolCallID:       toolResult.ToolCallID,
		Output:           toolResult.Result,
		ProviderExecuted: toolResult.ProviderExecuted,
		ProviderOptions:  fantasy.ProviderOptions(toolResult.ProviderMetadata),
	}
}

func sanitizeAnthropicProviderToolGuardMessages(
	ctx context.Context,
	logger slog.Logger,
	provider string,
	modelName string,
	messages []fantasy.Message,
	affectedMessages map[int]struct{},
	validationViolations int,
	extraFields ...slog.Field,
) []fantasy.Message {
	guardPrompt := invalidateProviderExecutedToolCallsInMessages(messages, affectedMessages)
	// Marking affected provider calls invalid lets the sanitizer remove the
	// unsafe history while preserving result payloads as plain text.
	sanitized, stats := SanitizeAnthropicProviderToolHistory(provider, guardPrompt)
	extra := []slog.Field{
		slog.F("validation_violations", validationViolations),
	}
	extra = append(extra, extraFields...)
	LogAnthropicProviderToolSanitization(
		ctx,
		logger,
		"pre_request_guard",
		provider,
		modelName,
		stats,
		extra...,
	)
	return sanitized
}

func isSafeAnthropicProviderToolPrompt(messages []fantasy.Message) bool {
	return len(ValidateAnthropicProviderToolHistory(messages)) == 0
}

func messageIndexesFromAnthropicProviderToolViolations(
	violations []AnthropicProviderToolHistoryViolation,
	messageCount int,
) map[int]struct{} {
	indexes := make(map[int]struct{})
	for _, violation := range violations {
		if violation.MessageIndex < 0 || violation.MessageIndex >= messageCount {
			continue
		}
		indexes[violation.MessageIndex] = struct{}{}
	}
	return indexes
}

func providerExecutedToolMessageIndexes(messages []fantasy.Message) map[int]struct{} {
	indexes := make(map[int]struct{})
	for messageIndex, message := range messages {
		for _, part := range message.Content {
			if toolCall, ok := safeMessageToolCallPart(part); ok && toolCall.ProviderExecuted {
				indexes[messageIndex] = struct{}{}
				break
			}
			if toolResult, ok := safeMessageToolResultPart(part); ok && toolResult.ProviderExecuted {
				indexes[messageIndex] = struct{}{}
				break
			}
		}
	}
	return indexes
}

func stripAnthropicProviderToolHistoryFromMessages(
	messages []fantasy.Message,
	affectedMessages map[int]struct{},
) ([]fantasy.Message, AnthropicProviderToolSanitizationStats) {
	var stats AnthropicProviderToolSanitizationStats
	if len(affectedMessages) == 0 {
		return messages, stats
	}

	out := make([]fantasy.Message, 0, len(messages))
	for messageIndex, message := range messages {
		if _, affected := affectedMessages[messageIndex]; !affected {
			out = appendSanitizedMessage(out, message)
			continue
		}

		parts := make([]fantasy.MessagePart, 0, len(message.Content))
		for _, part := range message.Content {
			if toolCall, ok := safeMessageToolCallPart(part); ok && toolCall.ProviderExecuted {
				stats.RemovedToolCalls++
				continue
			}
			if toolResult, ok := safeMessageToolResultPart(part); ok && toolResult.ProviderExecuted {
				stats.RemovedToolResults++
				if textPart, ok := AnthropicProviderToolResultTextPart(part); ok {
					parts = append(parts, textPart)
				}
				continue
			}
			parts = append(parts, part)
		}
		if len(parts) == 0 {
			stats.DroppedMessages++
			continue
		}
		message.Content = parts
		out = appendSanitizedMessage(out, message)
	}
	return out, stats
}

func appendSanitizedMessage(out []fantasy.Message, msg fantasy.Message) []fantasy.Message {
	if len(out) == 0 || out[len(out)-1].Role != msg.Role {
		return append(out, msg)
	}

	last := &out[len(out)-1]
	lastContent := applyMessageProviderOptionsToLastPart(last.Content, last.ProviderOptions)
	msgContent := applyMessageProviderOptionsToLastPart(msg.Content, msg.ProviderOptions)
	content := make([]fantasy.MessagePart, 0, len(lastContent)+len(msgContent))
	content = append(content, lastContent...)
	content = append(content, msgContent...)
	last.Content = content
	last.ProviderOptions = nil
	return out
}

func applyMessageProviderOptionsToLastPart(
	parts []fantasy.MessagePart,
	options fantasy.ProviderOptions,
) []fantasy.MessagePart {
	if len(options) == 0 || len(parts) == 0 {
		return parts
	}

	out := make([]fantasy.MessagePart, len(parts))
	copy(out, parts)
	lastIndex := len(out) - 1
	switch part := out[lastIndex].(type) {
	case fantasy.TextPart:
		part.ProviderOptions = mergeProviderOptions(part.ProviderOptions, options)
		out[lastIndex] = part
	case *fantasy.TextPart:
		if part != nil {
			clone := *part
			clone.ProviderOptions = mergeProviderOptions(clone.ProviderOptions, options)
			out[lastIndex] = &clone
		}
	case fantasy.ReasoningPart:
		part.ProviderOptions = mergeProviderOptions(part.ProviderOptions, options)
		out[lastIndex] = part
	case *fantasy.ReasoningPart:
		if part != nil {
			clone := *part
			clone.ProviderOptions = mergeProviderOptions(clone.ProviderOptions, options)
			out[lastIndex] = &clone
		}
	case fantasy.FilePart:
		part.ProviderOptions = mergeProviderOptions(part.ProviderOptions, options)
		out[lastIndex] = part
	case *fantasy.FilePart:
		if part != nil {
			clone := *part
			clone.ProviderOptions = mergeProviderOptions(clone.ProviderOptions, options)
			out[lastIndex] = &clone
		}
	case fantasy.ToolCallPart:
		part.ProviderOptions = mergeProviderOptions(part.ProviderOptions, options)
		out[lastIndex] = part
	case *fantasy.ToolCallPart:
		if part != nil {
			clone := *part
			clone.ProviderOptions = mergeProviderOptions(clone.ProviderOptions, options)
			out[lastIndex] = &clone
		}
	case fantasy.ToolResultPart:
		part.ProviderOptions = mergeProviderOptions(part.ProviderOptions, options)
		out[lastIndex] = part
	case *fantasy.ToolResultPart:
		if part != nil {
			clone := *part
			clone.ProviderOptions = mergeProviderOptions(clone.ProviderOptions, options)
			out[lastIndex] = &clone
		}
	}
	return out
}

func mergeProviderOptions(first, second fantasy.ProviderOptions) fantasy.ProviderOptions {
	if len(first) == 0 {
		return second
	}
	if len(second) == 0 {
		return first
	}

	merged := make(fantasy.ProviderOptions, len(first)+len(second))
	for provider, options := range first {
		merged[provider] = options
	}
	for provider, options := range second {
		if options != nil {
			merged[provider] = options
		}
	}
	return merged
}

func addAnthropicProviderToolSanitizationStats(
	first AnthropicProviderToolSanitizationStats,
	second AnthropicProviderToolSanitizationStats,
) AnthropicProviderToolSanitizationStats {
	return AnthropicProviderToolSanitizationStats{
		RemovedToolCalls:   first.RemovedToolCalls + second.RemovedToolCalls,
		RemovedToolResults: first.RemovedToolResults + second.RemovedToolResults,
		DroppedMessages:    first.DroppedMessages + second.DroppedMessages,
	}
}

func anthropicProviderToolViolationLogDetails(
	violations []AnthropicProviderToolHistoryViolation,
) ([]map[string]any, bool) {
	count := min(len(violations), maxAnthropicProviderToolViolationLogDetails)
	details := make([]map[string]any, 0, count)
	for _, violation := range violations[:count] {
		details = append(details, map[string]any{
			"message_index": violation.MessageIndex,
			"part_index":    violation.PartIndex,
			"id":            violation.ID,
			"reason":        violation.Reason,
		})
	}
	return details, len(violations) > maxAnthropicProviderToolViolationLogDetails
}

func invalidateProviderExecutedToolCallsInMessages(
	messages []fantasy.Message,
	affectedMessages map[int]struct{},
) []fantasy.Message {
	if len(affectedMessages) == 0 {
		return messages
	}
	out := make([]fantasy.Message, len(messages))
	copy(out, messages)
	for messageIndex := range affectedMessages {
		if messageIndex < 0 || messageIndex >= len(out) {
			continue
		}
		message := out[messageIndex]
		if len(message.Content) == 0 {
			continue
		}
		parts := make([]fantasy.MessagePart, len(message.Content))
		for partIndex, part := range message.Content {
			parts[partIndex] = invalidateProviderExecutedToolCallPart(part)
		}
		message.Content = parts
		out[messageIndex] = message
	}
	return out
}

func invalidateProviderExecutedToolCallPart(part fantasy.MessagePart) fantasy.MessagePart {
	switch value := part.(type) {
	case fantasy.ToolCallPart:
		if value.ProviderExecuted {
			value.ToolName = ""
		}
		return value
	case *fantasy.ToolCallPart:
		if value == nil {
			return part
		}
		clone := *value
		if clone.ProviderExecuted {
			clone.ToolName = ""
		}
		return &clone
	default:
		return part
	}
}

func safeMessageToolCallPart(part fantasy.MessagePart) (fantasy.ToolCallPart, bool) {
	var zero fantasy.ToolCallPart
	if part == nil {
		return zero, false
	}
	if value, ok := part.(*fantasy.ToolCallPart); ok && value == nil {
		return zero, false
	}
	type toolCallPart = fantasy.ToolCallPart
	return fantasy.AsMessagePart[toolCallPart](part)
}

func safeMessageToolResultPart(part fantasy.MessagePart) (fantasy.ToolResultPart, bool) {
	var zero fantasy.ToolResultPart
	if part == nil {
		return zero, false
	}
	if value, ok := part.(*fantasy.ToolResultPart); ok && value == nil {
		return zero, false
	}
	type toolResultPart = fantasy.ToolResultPart
	return fantasy.AsMessagePart[toolResultPart](part)
}
