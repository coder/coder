package chatd

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatcost"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
)

const interruptedToolResultErrorMessage = "tool call was interrupted before it produced a result"

type buildCommitStepMessagesInput struct {
	modelConfigID      uuid.UUID
	modelCallConfig    codersdk.ChatModelCallConfig
	step               stepData
	toolNameToConfigID map[string]uuid.UUID
	logger             slog.Logger
	contentVersion     int16
}

type stepMessagesForCommit struct {
	Messages       []chatstate.Message
	VisibleIndexes []int
}

func buildCommitStepMessages(input buildCommitStepMessagesInput) (stepMessagesForCommit, error) {
	contentVersion := input.contentVersion
	if contentVersion == 0 {
		contentVersion = chatprompt.CurrentContentVersion
	}

	assistantBlocks, toolResults := splitStepContent(input.step.Content)
	assistantParts := buildAssistantParts(input.logger, assistantBlocks, toolResults, input.step, input.toolNameToConfigID)

	messages := make([]chatstate.Message, 0, 1+len(toolResults))
	if len(assistantParts) > 0 {
		assistantContent, err := chatprompt.MarshalParts(assistantParts)
		if err != nil {
			return stepMessagesForCommit{}, xerrors.Errorf("marshal assistant content: %w", err)
		}
		messages = append(messages, assistantMessage(input.modelConfigID, contentVersion, assistantContent, input.step, input.modelCallConfig))
	}

	for _, toolResult := range toolResults {
		part := chatprompt.PartFromContentWithLogger(context.Background(), input.logger, toolResult)
		applyToolMetadata(&part, input.toolNameToConfigID)
		if part.ToolCallID != "" && input.step.ToolResultCreatedAt != nil {
			if ts, ok := input.step.ToolResultCreatedAt[part.ToolCallID]; ok {
				part.CreatedAt = &ts
			}
		}
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{part})
		if err != nil {
			return stepMessagesForCommit{}, xerrors.Errorf("marshal tool result: %w", err)
		}
		messages = append(messages, baseMessage(database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, input.modelConfigID, contentVersion, content))
	}

	return stepMessagesForCommit{
		Messages:       messages,
		VisibleIndexes: visibleMessageIndexes(messages),
	}, nil
}

func splitStepContent(content []fantasy.Content) ([]fantasy.Content, []fantasy.ToolResultContent) {
	assistantBlocks := make([]fantasy.Content, 0, len(content))
	toolResults := make([]fantasy.ToolResultContent, 0)
	for _, block := range content {
		if tr, ok := asToolResultContent(block); ok && !tr.ProviderExecuted {
			toolResults = append(toolResults, tr)
			continue
		}
		assistantBlocks = append(assistantBlocks, block)
	}
	return assistantBlocks, toolResults
}

func asToolResultContent(block fantasy.Content) (fantasy.ToolResultContent, bool) {
	if tr, ok := fantasy.AsContentType[fantasy.ToolResultContent](block); ok {
		return tr, true
	}
	if tr, ok := fantasy.AsContentType[*fantasy.ToolResultContent](block); ok && tr != nil {
		return *tr, true
	}
	return fantasy.ToolResultContent{}, false
}

func buildAssistantParts(
	logger slog.Logger,
	assistantBlocks []fantasy.Content,
	toolResults []fantasy.ToolResultContent,
	step stepData,
	toolNameToConfigID map[string]uuid.UUID,
) []codersdk.ChatMessagePart {
	parts := make([]codersdk.ChatMessagePart, 0, len(assistantBlocks)+len(toolResults))
	reasoningIdx := 0
	for _, block := range assistantBlocks {
		part := chatprompt.PartFromContentWithLogger(context.Background(), logger, block)
		applyToolMetadata(&part, toolNameToConfigID)
		switch part.Type {
		case codersdk.ChatMessagePartTypeToolCall:
			if part.ToolCallID != "" && step.ToolCallCreatedAt != nil {
				if ts, ok := step.ToolCallCreatedAt[part.ToolCallID]; ok {
					part.CreatedAt = &ts
				}
			}
		case codersdk.ChatMessagePartTypeToolResult:
			if part.ToolCallID != "" && step.ToolResultCreatedAt != nil {
				if ts, ok := step.ToolResultCreatedAt[part.ToolCallID]; ok {
					part.CreatedAt = &ts
				}
			}
		case codersdk.ChatMessagePartTypeReasoning:
			if reasoningIdx < len(step.ReasoningStartedAt) {
				if ts := step.ReasoningStartedAt[reasoningIdx]; !ts.IsZero() {
					part.CreatedAt = &ts
				}
			}
			if reasoningIdx < len(step.ReasoningCompletedAt) {
				if ts := step.ReasoningCompletedAt[reasoningIdx]; !ts.IsZero() {
					part.CompletedAt = &ts
				}
			}
			reasoningIdx++
		}
		if part.Type != "" {
			parts = append(parts, part)
		}
	}
	for _, tr := range toolResults {
		attachments, err := chattool.AttachmentsFromMetadata(tr.ClientMetadata)
		if err != nil {
			logger.Warn(context.Background(), "skipping malformed tool attachment metadata",
				slog.F("tool_name", tr.ToolName),
				slog.F("tool_call_id", tr.ToolCallID),
				slog.Error(err),
			)
			continue
		}
		for _, attachment := range attachments {
			parts = append(parts, codersdk.ChatMessageFile(attachment.FileID, attachment.MediaType, attachment.Name))
		}
	}
	return parts
}

func applyToolMetadata(part *codersdk.ChatMessagePart, toolNameToConfigID map[string]uuid.UUID) {
	if part.ToolName == "" || len(toolNameToConfigID) == 0 {
		return
	}
	if configID, ok := toolNameToConfigID[part.ToolName]; ok {
		part.MCPServerConfigID = uuid.NullUUID{UUID: configID, Valid: true}
	}
}

func assistantMessage(
	modelConfigID uuid.UUID,
	contentVersion int16,
	content pqtype.NullRawMessage,
	step stepData,
	modelCallConfig codersdk.ChatModelCallConfig,
) chatstate.Message {
	msg := baseMessage(database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, modelConfigID, contentVersion, content)
	if step.Usage != (fantasy.Usage{}) {
		msg.InputTokens = nullInt64IfNonZero(step.Usage.InputTokens)
		msg.OutputTokens = nullInt64IfNonZero(step.Usage.OutputTokens)
		msg.TotalTokens = nullInt64IfNonZero(step.Usage.TotalTokens)
		msg.ReasoningTokens = nullInt64IfNonZero(step.Usage.ReasoningTokens)
		msg.CacheCreationTokens = nullInt64IfNonZero(step.Usage.CacheCreationTokens)
		msg.CacheReadTokens = nullInt64IfNonZero(step.Usage.CacheReadTokens)
		usage := codersdk.ChatMessageUsage{
			InputTokens:         int64PtrIfNonZero(step.Usage.InputTokens),
			OutputTokens:        int64PtrIfNonZero(step.Usage.OutputTokens),
			ReasoningTokens:     int64PtrIfNonZero(step.Usage.ReasoningTokens),
			CacheCreationTokens: int64PtrIfNonZero(step.Usage.CacheCreationTokens),
			CacheReadTokens:     int64PtrIfNonZero(step.Usage.CacheReadTokens),
		}
		if totalCost := chatcost.CalculateTotalCostMicros(usage, modelCallConfig.Cost); totalCost != nil {
			msg.TotalCostMicros = sql.NullInt64{Int64: *totalCost, Valid: true}
		}
	}
	msg.ContextLimit = step.ContextLimit
	if step.Runtime > 0 {
		msg.RuntimeMs = sql.NullInt64{Int64: step.Runtime.Milliseconds(), Valid: true}
	}
	return msg
}

func baseMessage(
	role database.ChatMessageRole,
	visibility database.ChatMessageVisibility,
	modelConfigID uuid.UUID,
	contentVersion int16,
	content pqtype.NullRawMessage,
) chatstate.Message {
	return chatstate.Message{
		Role:           role,
		Content:        content,
		Visibility:     visibility,
		ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: modelConfigID != uuid.Nil},
		ContentVersion: contentVersion,
	}
}

func nullInt64IfNonZero(value int64) sql.NullInt64 {
	if value == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: value, Valid: true}
}

func int64PtrIfNonZero(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func visibleMessageIndexes(messages []chatstate.Message) []int {
	indexes := make([]int, 0, len(messages))
	for i, msg := range messages {
		if msg.Visibility == database.ChatMessageVisibilityBoth || msg.Visibility == database.ChatMessageVisibilityUser {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func textFromParts(parts []codersdk.ChatMessagePart) string {
	var builder strings.Builder
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeText {
			_, _ = builder.WriteString(part.Text)
		}
	}
	return builder.String()
}

type buildCompactionMessagesInput struct {
	modelConfigID  uuid.UUID
	activeAPIKeyID string
	toolCallID     string
	toolName       string
	compaction     compactionOutcome
	contentVersion int16
}

type compactionMessagesForCommit struct {
	Messages    []chatstate.Message
	HiddenCount int
}

func buildCompactionMessages(input buildCompactionMessagesInput) (compactionMessagesForCommit, error) {
	contentVersion := input.contentVersion
	if contentVersion == 0 {
		contentVersion = chatprompt.CurrentContentVersion
	}
	toolName := input.toolName
	if toolName == "" {
		toolName = "chat_summarized"
	}

	systemContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(input.compaction.SystemSummary)})
	if err != nil {
		return compactionMessagesForCommit{}, xerrors.Errorf("marshal compaction system summary: %w", err)
	}
	args, err := json.Marshal(map[string]any{
		"source":            "automatic",
		"threshold_percent": input.compaction.ThresholdPercent,
	})
	if err != nil {
		return compactionMessagesForCommit{}, xerrors.Errorf("marshal compaction args: %w", err)
	}
	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolCall(input.toolCallID, toolName, args),
	})
	if err != nil {
		return compactionMessagesForCommit{}, xerrors.Errorf("marshal compaction tool call: %w", err)
	}
	summaryResult, err := json.Marshal(map[string]any{
		"summary":              input.compaction.SummaryReport,
		"source":               "automatic",
		"threshold_percent":    input.compaction.ThresholdPercent,
		"usage_percent":        input.compaction.UsagePercent,
		"context_tokens":       input.compaction.ContextTokens,
		"context_limit_tokens": input.compaction.ContextLimit,
	})
	if err != nil {
		return compactionMessagesForCommit{}, xerrors.Errorf("marshal compaction result: %w", err)
	}
	toolContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolResult(input.toolCallID, toolName, summaryResult, false, false),
	})
	if err != nil {
		return compactionMessagesForCommit{}, xerrors.Errorf("marshal compaction tool result: %w", err)
	}

	messages := []chatstate.Message{
		{
			Role:           database.ChatMessageRoleUser,
			Content:        systemContent,
			Visibility:     database.ChatMessageVisibilityModel,
			ModelConfigID:  uuid.NullUUID{UUID: input.modelConfigID, Valid: input.modelConfigID != uuid.Nil},
			ContentVersion: contentVersion,
			APIKeyID:       sql.NullString{String: input.activeAPIKeyID, Valid: input.activeAPIKeyID != ""},
		},
		baseMessage(database.ChatMessageRoleAssistant, database.ChatMessageVisibilityUser, input.modelConfigID, contentVersion, assistantContent),
		baseMessage(database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, input.modelConfigID, contentVersion, toolContent),
	}
	for i := range messages {
		messages[i].Compressed = true
	}
	return compactionMessagesForCommit{Messages: messages, HiddenCount: 1}, nil
}

func currentTurnStepCount(messages []database.ChatMessage) int {
	latestUser := -1
	for i, msg := range messages {
		if msg.Deleted || msg.Compressed {
			continue
		}
		if msg.Role == database.ChatMessageRoleUser {
			latestUser = i
		}
	}
	count := 0
	for i := latestUser + 1; i < len(messages); i++ {
		msg := messages[i]
		if msg.Deleted || msg.Compressed {
			continue
		}
		if msg.Role == database.ChatMessageRoleAssistant {
			count++
		}
	}
	return count
}

type compactionRequirement int

const (
	compactionRequirementNotNeeded compactionRequirement = iota
	compactionRequirementNeeded
)

func compactionStatusFromHistory(
	messages []database.ChatMessage,
	requirement compactionRequirement,
	thresholdPercent int32,
	contextLimit int64,
) compactionStatus {
	boundaryIndex := latestCompactionBoundaryIndex(messages)
	if requirement == compactionRequirementNeeded {
		if boundaryIndex == -1 {
			return compactionStatusNeeded
		}
		// The first assistant response after the previously compacted summary.
		// Messages with role ChatMessageRoleAssistant carry context usage.
		// Looking at ChatMessageRoleAssistant is enough - ChatMessageRoleTool
		// does not carry context usage, and is always preceded by an assistant
		// message.
		if assistant, ok := firstUncompressedAssistantAfter(messages, boundaryIndex); ok &&
			postCompactionAssistantOverLimit(assistant, thresholdPercent, contextLimit) {
			return compactionStatusStillOverLimit
		}
		if hasUncompressedMessageAfter(messages, boundaryIndex) {
			return compactionStatusNeeded
		}
		return compactionStatusAfterCompaction
	}
	if boundaryIndex != -1 && !hasUncompressedMessageAfter(messages, boundaryIndex) {
		return compactionStatusAfterCompaction
	}
	return compactionStatusNotNeeded
}

func latestCompactionBoundaryIndex(messages []database.ChatMessage) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if isCompactionBoundaryMessage(messages[i]) {
			return i
		}
	}
	return -1
}

func isCompactionBoundaryMessage(msg database.ChatMessage) bool {
	if msg.Deleted || !msg.Compressed {
		return false
	}
	parts, err := chatprompt.ParseContent(msg)
	if err != nil {
		return false
	}
	for _, part := range parts {
		if part.ToolName == "chat_summarized" &&
			(part.Type == codersdk.ChatMessagePartTypeToolCall || part.Type == codersdk.ChatMessagePartTypeToolResult) {
			return true
		}
	}
	return false
}

func firstUncompressedAssistantAfter(messages []database.ChatMessage, index int) (database.ChatMessage, bool) {
	for i := index + 1; i < len(messages); i++ {
		msg := messages[i]
		if msg.Deleted || msg.Compressed {
			continue
		}
		if msg.Role == database.ChatMessageRoleAssistant {
			return msg, true
		}
	}
	return database.ChatMessage{}, false
}

func hasUncompressedMessageAfter(messages []database.ChatMessage, index int) bool {
	for i := index + 1; i < len(messages); i++ {
		msg := messages[i]
		if !msg.Deleted && !msg.Compressed {
			return true
		}
	}
	return false
}

func postCompactionAssistantOverLimit(msg database.ChatMessage, thresholdPercent int32, contextLimit int64) bool {
	return shouldCompactPromptUsage(usageFromMessage(msg), contextLimit, thresholdPercent)
}

func usageFromMessage(msg database.ChatMessage) fantasy.Usage {
	var usage fantasy.Usage
	if msg.InputTokens.Valid {
		usage.InputTokens = msg.InputTokens.Int64
	}
	if msg.OutputTokens.Valid {
		usage.OutputTokens = msg.OutputTokens.Int64
	}
	if msg.TotalTokens.Valid {
		usage.TotalTokens = msg.TotalTokens.Int64
	}
	if msg.ReasoningTokens.Valid {
		usage.ReasoningTokens = msg.ReasoningTokens.Int64
	}
	if msg.CacheCreationTokens.Valid {
		usage.CacheCreationTokens = msg.CacheCreationTokens.Int64
	}
	if msg.CacheReadTokens.Valid {
		usage.CacheReadTokens = msg.CacheReadTokens.Int64
	}
	return usage
}

func historyHasStopAfterToolResult(messages []database.ChatMessage, stopAfterTools map[string]struct{}) (bool, error) {
	if len(stopAfterTools) == 0 {
		return false, nil
	}
	start := 0
	for i, msg := range messages {
		if msg.Deleted || msg.Compressed {
			continue
		}
		if msg.Role == database.ChatMessageRoleUser {
			start = i + 1
		}
	}
	for _, msg := range messages[start:] {
		if msg.Deleted || msg.Compressed || msg.Role != database.ChatMessageRoleTool {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		if err != nil {
			return false, xerrors.Errorf("parse tool message: %w", err)
		}
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeToolResult || part.IsError {
				continue
			}
			if _, ok := stopAfterTools[part.ToolName]; ok {
				return true, nil
			}
		}
	}
	return false, nil
}

func currentHistoryComplete(messages []database.ChatMessage) (bool, error) {
	idx := lastMessageIndex(messages, func(database.ChatMessage) bool { return true })
	if idx == -1 || messages[idx].Role != database.ChatMessageRoleAssistant {
		return false, nil
	}
	parts, err := chatprompt.ParseContent(messages[idx])
	if err != nil {
		return false, xerrors.Errorf("parse latest assistant message: %w", err)
	}
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeToolCall && !part.ProviderExecuted {
			return false, nil
		}
	}
	return true, nil
}

func lastMessageIndex(messages []database.ChatMessage, accept func(database.ChatMessage) bool) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Deleted || messages[i].Compressed {
			continue
		}
		if accept(messages[i]) {
			return i
		}
	}
	return -1
}

func handledToolCallIDs(messages []database.ChatMessage) (map[string]bool, error) {
	handled := make(map[string]bool)
	for _, msg := range messages {
		if msg.Deleted || msg.Compressed || msg.Role != database.ChatMessageRoleTool {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		if err != nil {
			return nil, xerrors.Errorf("parse tool message: %w", err)
		}
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeToolResult && part.ToolCallID != "" {
				handled[part.ToolCallID] = true
			}
		}
	}
	return handled, nil
}

type bufferedPartsToPartialMessagesInput struct {
	parts          []messagepartbuffer.Part
	modelConfigID  uuid.UUID
	contentVersion int16
	logger         slog.Logger
	interruptedAt  time.Time
}

type partialToolCall struct {
	part      codersdk.ChatMessagePart
	index     int
	argsDelta strings.Builder
	valid     bool
	durable   bool
}

type partialToolResult struct {
	part        codersdk.ChatMessagePart
	resultDelta strings.Builder
	completed   bool
}

func bufferedPartsToPartialMessages(input bufferedPartsToPartialMessagesInput) ([]chatstate.Message, error) {
	contentVersion := input.contentVersion
	if contentVersion == 0 {
		contentVersion = chatprompt.CurrentContentVersion
	}
	parts := slices.Clone(input.parts)
	slices.SortFunc(parts, func(a, b messagepartbuffer.Part) int {
		return cmp.Compare(a.Seq, b.Seq)
	})

	state := partialMessageConversionState{
		input:          input,
		contentVersion: contentVersion,
		toolCalls:      make(map[string]*partialToolCall),
		toolResults:    make(map[string]*partialToolResult),
		answered:       make(map[string]bool),
	}
	for _, buffered := range parts {
		if err := state.consume(buffered); err != nil {
			return nil, err
		}
	}
	if err := state.finalizeToolCallPlaceholders(); err != nil {
		return nil, err
	}
	if err := state.flushAssistant(); err != nil {
		return nil, err
	}
	if err := state.flushAccumulatedToolResults(); err != nil {
		return nil, err
	}
	if err := state.appendSyntheticInterruptionResults(); err != nil {
		return nil, err
	}
	return state.messages, nil
}

type partialMessageConversionState struct {
	input          bufferedPartsToPartialMessagesInput
	contentVersion int16

	messages        []chatstate.Message
	assistantParts  []codersdk.ChatMessagePart
	toolCalls       map[string]*partialToolCall
	toolCallOrder   []string
	toolResults     map[string]*partialToolResult
	toolResultOrder []string
	answered        map[string]bool
}

func (s *partialMessageConversionState) consume(buffered messagepartbuffer.Part) error {
	switch buffered.Role {
	case codersdk.ChatMessageRoleAssistant:
		s.consumeAssistantPart(buffered)
	case codersdk.ChatMessageRoleTool:
		return s.consumeToolPart(buffered)
	default:
		s.logSkippedPart(buffered, "unsupported buffered part role")
	}
	return nil
}

func (s *partialMessageConversionState) consumeAssistantPart(buffered messagepartbuffer.Part) {
	part := buffered.MessagePart
	if part.Type == "" {
		s.logSkippedPart(buffered, "empty buffered assistant part type")
		return
	}
	if part.Type != codersdk.ChatMessagePartTypeToolCall {
		if part.Type == codersdk.ChatMessagePartTypeReasoning &&
			!s.input.interruptedAt.IsZero() {
			interruptedAt := s.input.interruptedAt
			if part.CreatedAt == nil {
				part.CreatedAt = &interruptedAt
			}
			if part.CompletedAt == nil {
				part.CompletedAt = &interruptedAt
			}
		}
		s.assistantParts = append(s.assistantParts, part)
		return
	}
	if part.ToolCallID == "" {
		s.logSkippedPart(buffered, "tool call part missing tool call ID")
		return
	}
	call := s.toolCall(part.ToolCallID)
	call.part.Type = codersdk.ChatMessagePartTypeToolCall
	call.part.ToolCallID = part.ToolCallID
	if part.ToolName != "" {
		call.part.ToolName = part.ToolName
	}
	if part.MCPServerConfigID.Valid {
		call.part.MCPServerConfigID = part.MCPServerConfigID
	}
	if part.CreatedAt != nil {
		call.part.CreatedAt = part.CreatedAt
	}
	call.part.ProviderExecuted = call.part.ProviderExecuted || part.ProviderExecuted

	if part.ArgsDelta != "" {
		if call.durable {
			s.logSkippedPart(buffered, "tool call args delta arrived after full tool call")
			return
		}
		_, _ = call.argsDelta.WriteString(part.ArgsDelta)
		return
	}

	durable := part
	durable.ArgsDelta = ""
	if len(durable.Args) > 0 && !json.Valid(durable.Args) {
		call.valid = false
		s.assistantParts[call.index] = codersdk.ChatMessagePart{}
		s.logSkippedPart(buffered, "tool call part has invalid durable args")
		return
	}
	if call.durable {
		s.logSkippedPart(buffered, "duplicate durable tool call part")
	}
	call.part = durable
	call.valid = true
	call.durable = true
	s.assistantParts[call.index] = durable
}

func (s *partialMessageConversionState) consumeToolPart(buffered messagepartbuffer.Part) error {
	part := buffered.MessagePart
	if part.Type != codersdk.ChatMessagePartTypeToolResult {
		s.logSkippedPart(buffered, "non tool-result part with tool role")
		return nil
	}
	if part.ToolCallID == "" {
		s.logSkippedPart(buffered, "tool result part missing tool call ID")
		return nil
	}
	if part.ResultReset {
		result := s.toolResult(part.ToolCallID)
		result.part.ToolCallID = part.ToolCallID
		result.part.ToolName = part.ToolName
		result.resultDelta.Reset()
		s.logSkippedPart(buffered, "streaming tool result reset is not durable")
		return nil
	}
	if part.ResultDelta != "" {
		result := s.toolResult(part.ToolCallID)
		result.part.ToolCallID = part.ToolCallID
		if part.ToolName != "" {
			result.part.ToolName = part.ToolName
		}
		if part.MCPServerConfigID.Valid {
			result.part.MCPServerConfigID = part.MCPServerConfigID
		}
		if part.CreatedAt != nil {
			result.part.CreatedAt = part.CreatedAt
		}
		result.part.ProviderExecuted = result.part.ProviderExecuted || part.ProviderExecuted
		_, _ = result.resultDelta.WriteString(part.ResultDelta)
		return nil
	}
	if err := s.finalizeToolCallPlaceholders(); err != nil {
		return err
	}
	if !s.toolCallDurable(part.ToolCallID) {
		s.logSkippedPart(buffered, "tool result has no matching durable tool call")
		return nil
	}
	if len(part.Result) == 0 || !json.Valid(part.Result) {
		s.logSkippedPart(buffered, "tool result part has invalid durable result")
		return nil
	}
	if s.answered[part.ToolCallID] {
		s.logSkippedPart(buffered, "duplicate durable tool result part")
		return nil
	}
	part.ResultDelta = ""
	part.ResultReset = false
	if err := s.flushAssistant(); err != nil {
		return err
	}
	if err := s.appendToolResult(part); err != nil {
		return err
	}
	s.answered[part.ToolCallID] = true
	return nil
}

func (s *partialMessageConversionState) toolCall(id string) *partialToolCall {
	call := s.toolCalls[id]
	if call != nil {
		return call
	}
	call = &partialToolCall{index: len(s.assistantParts), valid: true}
	s.toolCalls[id] = call
	s.toolCallOrder = append(s.toolCallOrder, id)
	s.assistantParts = append(s.assistantParts, codersdk.ChatMessagePart{})
	return call
}

func (s *partialMessageConversionState) toolResult(id string) *partialToolResult {
	result := s.toolResults[id]
	if result != nil {
		return result
	}
	result = &partialToolResult{}
	s.toolResults[id] = result
	s.toolResultOrder = append(s.toolResultOrder, id)
	return result
}

func (s *partialMessageConversionState) finalizeToolCallPlaceholders() error {
	for _, id := range s.toolCallOrder {
		call := s.toolCalls[id]
		if call == nil || call.durable || !call.valid {
			continue
		}
		args := json.RawMessage(call.argsDelta.String())
		if len(args) == 0 || !json.Valid(args) {
			s.assistantParts[call.index] = codersdk.ChatMessagePart{}
			call.valid = false
			s.logSkippedPart(messagepartbuffer.Part{
				Role:        codersdk.ChatMessageRoleAssistant,
				MessagePart: call.part,
			}, "tool call args delta did not form durable JSON")
			continue
		}
		call.part.Args = args
		call.part.ArgsDelta = ""
		call.durable = true
		s.assistantParts[call.index] = call.part
	}
	return nil
}

func (s *partialMessageConversionState) flushAssistant() error {
	if len(s.assistantParts) == 0 {
		return nil
	}
	durable := make([]codersdk.ChatMessagePart, 0, len(s.assistantParts))
	for _, part := range s.assistantParts {
		if part.Type == "" {
			continue
		}
		part.ArgsDelta = ""
		part.ResultDelta = ""
		part.ResultReset = false
		durable = append(durable, part)
	}
	s.assistantParts = nil
	if len(durable) == 0 {
		return nil
	}
	content, err := chatprompt.MarshalParts(durable)
	if err != nil {
		return xerrors.Errorf("marshal partial assistant: %w", err)
	}
	s.messages = append(s.messages, baseMessage(database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, s.input.modelConfigID, s.contentVersion, content))
	return nil
}

func (s *partialMessageConversionState) flushAccumulatedToolResults() error {
	for _, id := range s.toolResultOrder {
		if s.answered[id] {
			continue
		}
		result := s.toolResults[id]
		if result == nil || result.completed {
			continue
		}
		if result.resultDelta.Len() == 0 {
			continue
		}
		s.logSkippedPart(messagepartbuffer.Part{Role: codersdk.ChatMessageRoleTool, MessagePart: result.part}, "streaming tool result delta is not durable")
	}
	return nil
}

func (s *partialMessageConversionState) appendToolResult(part codersdk.ChatMessagePart) error {
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{part})
	if err != nil {
		return xerrors.Errorf("marshal partial tool result: %w", err)
	}
	s.messages = append(s.messages, baseMessage(database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, s.input.modelConfigID, s.contentVersion, content))
	return nil
}

func (s *partialMessageConversionState) appendSyntheticInterruptionResults() error {
	for _, id := range s.toolCallOrder {
		if s.answered[id] {
			continue
		}
		call := s.toolCalls[id]
		if call == nil || !call.valid || !call.durable || call.part.ProviderExecuted {
			continue
		}
		result, err := json.Marshal(map[string]string{"error": interruptedToolResultErrorMessage})
		if err != nil {
			return xerrors.Errorf("marshal synthetic interruption result: %w", err)
		}
		part := codersdk.ChatMessageToolResult(call.part.ToolCallID, call.part.ToolName, result, true, false)
		part.MCPServerConfigID = call.part.MCPServerConfigID
		if !s.input.interruptedAt.IsZero() {
			part.CreatedAt = &s.input.interruptedAt
		}
		if err := s.appendToolResult(part); err != nil {
			return xerrors.Errorf("marshal synthetic interruption message: %w", err)
		}
		s.answered[id] = true
	}
	return nil
}

func (s *partialMessageConversionState) toolCallDurable(id string) bool {
	call := s.toolCalls[id]
	return call != nil && call.valid && call.durable
}

func (s *partialMessageConversionState) logSkippedPart(buffered messagepartbuffer.Part, reason string) {
	s.input.logger.Warn(context.Background(), "skipping buffered chat message part",
		slog.F("reason", reason),
		slog.F("role", buffered.Role),
		slog.F("part_type", buffered.MessagePart.Type),
		slog.F("tool_call_id", buffered.MessagePart.ToolCallID),
		slog.F("tool_name", buffered.MessagePart.ToolName),
	)
}
