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
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/messagepartbuffer"
	"github.com/coder/coder/v2/codersdk"
)

const interruptedToolResultErrorMessage = "tool call was interrupted before it produced a result"

type buildCommitStepMessagesInput struct {
	modelConfigID          uuid.UUID
	step                   stepData
	toolNameToConfigID     map[string]uuid.UUID
	logger                 slog.Logger
	contentVersion         int16
	hookRewrittenToolCalls map[string]json.RawMessage
}

type stepMessagesForCommit struct {
	Messages       []chatstate.Message
	VisibleIndexes []int
	// ConsumeCompactionRequest clears the manual compaction marker
	// atomically with the commit. Set on compaction commits.
	ConsumeCompactionRequest bool
}

func buildCommitStepMessages(input buildCommitStepMessagesInput) (stepMessagesForCommit, error) {
	contentVersion := input.contentVersion
	if contentVersion == 0 {
		contentVersion = chatprompt.CurrentContentVersion
	}

	assistantBlocks, toolResults := splitStepContent(input.step.Content)
	assistantParts := buildAssistantParts(input.logger, assistantBlocks, toolResults, input.step, input.toolNameToConfigID, input.hookRewrittenToolCalls)

	messages := make([]chatstate.Message, 0, 1+len(toolResults))
	if len(assistantParts) > 0 {
		assistantContent, err := chatprompt.MarshalParts(assistantParts)
		if err != nil {
			return stepMessagesForCommit{}, xerrors.Errorf("marshal assistant content: %w", err)
		}
		messages = append(messages, assistantMessage(input.modelConfigID, contentVersion, assistantContent, input.step))
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

	// Usage sums runtime_ms across rows, so the batch window is billed
	// once on a dedicated record instead of an arbitrary member row.
	stamp, ok, err := batchUsageMessage(input.modelConfigID, contentVersion, input.step.BatchRuntime, input.step.BatchBilledCalls)
	if err != nil {
		return stepMessagesForCommit{}, err
	}
	if ok {
		messages = append(messages, stamp)
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
	hookRewrittenToolCalls map[string]json.RawMessage,
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
			// Hooks never see provider-executed calls, so such a call must not
			// inherit attribution from an ordinary call that reused its ID.
			if part.ToolCallID != "" && !part.ProviderExecuted {
				_, part.HookRewritten = hookRewrittenToolCalls[part.ToolCallID]
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
) chatstate.Message {
	msg := baseMessage(database.ChatMessageRoleAssistant, database.ChatMessageVisibilityBoth, modelConfigID, contentVersion, content)
	if step.Usage != (fantasy.Usage{}) {
		msg.InputTokens = nullInt64IfNonZero(step.Usage.InputTokens)
		msg.OutputTokens = nullInt64IfNonZero(step.Usage.OutputTokens)
		msg.TotalTokens = nullInt64IfNonZero(step.Usage.TotalTokens)
		msg.ReasoningTokens = nullInt64IfNonZero(step.Usage.ReasoningTokens)
		msg.CacheCreationTokens = nullInt64IfNonZero(step.Usage.CacheCreationTokens)
		msg.CacheReadTokens = nullInt64IfNonZero(step.Usage.CacheReadTokens)
	}
	msg.ContextLimit = step.ContextLimit
	// InsertChatMessages maps a zero runtime to NULL, so a model
	// invocation shorter than a millisecond persists the same way an
	// unmeasured one does.
	msg.RuntimeMs = nullInt64IfNonZero(step.Runtime.Milliseconds())
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

// toolBatchUsagePartType marks the dedicated billing record for a local
// tool batch. Internal to chatd: the row is persisted with model
// visibility so it never reaches the API or SSE, and prompt replay drops
// it because the part converts to no provider content.
const toolBatchUsagePartType codersdk.ChatMessagePartType = "tool-batch-usage"

// toolBatchUsagePayload is the audit payload stored on the usage record.
// It duplicates the row's runtime_ms so the billed window survives in
// content for debugging, alongside how many call intervals produced it.
type toolBatchUsagePayload struct {
	BilledMs    int64 `json:"billed_ms"`
	BilledCalls int   `json:"billed_calls"`
}

// batchUsageMessage builds the single model-invisible row that carries a
// local tool batch's billed runtime. Usage sums runtime_ms across rows,
// so a dedicated record keeps real tool results free of batch-level
// runtime. Completed and interrupted batches share this helper. Returns
// false when the batch bills no whole millisecond.
func batchUsageMessage(
	modelConfigID uuid.UUID,
	contentVersion int16,
	runtime time.Duration,
	billedCalls int,
) (chatstate.Message, bool, error) {
	runtimeMs := runtime.Milliseconds()
	if runtimeMs <= 0 {
		return chatstate.Message{}, false, nil
	}
	payload, err := json.Marshal(toolBatchUsagePayload{
		BilledMs:    runtimeMs,
		BilledCalls: billedCalls,
	})
	if err != nil {
		return chatstate.Message{}, false, xerrors.Errorf("marshal tool batch usage payload: %w", err)
	}
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{{
		Type:   toolBatchUsagePartType,
		Result: payload,
	}})
	if err != nil {
		return chatstate.Message{}, false, xerrors.Errorf("marshal tool batch usage part: %w", err)
	}
	msg := baseMessage(database.ChatMessageRoleTool, database.ChatMessageVisibilityModel, modelConfigID, contentVersion, content)
	msg.RuntimeMs = sql.NullInt64{Int64: runtimeMs, Valid: true}
	return msg, true, nil
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
	modelConfigID       uuid.UUID
	toolCallID          string
	toolName            string
	compaction          compactionOutcome
	contentVersion      int16
	pendingUserMessages []database.ChatMessage
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
	source := input.compaction.Source
	if source == "" {
		source = chatloop.CompactionSourceAutomatic
	}
	args, err := json.Marshal(map[string]any{
		"source":            source,
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
		"source":               source,
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

	assistantMsg := baseMessage(database.ChatMessageRoleAssistant, database.ChatMessageVisibilityUser, input.modelConfigID, contentVersion, assistantContent)
	assistantMsg.RuntimeMs = nullInt64IfNonZero(input.compaction.Runtime.Milliseconds())
	messages := []chatstate.Message{
		{
			Role:           database.ChatMessageRoleUser,
			Content:        systemContent,
			Visibility:     database.ChatMessageVisibilityModel,
			ModelConfigID:  uuid.NullUUID{UUID: input.modelConfigID, Valid: input.modelConfigID != uuid.Nil},
			ContentVersion: contentVersion,
		},
		assistantMsg,
		baseMessage(database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, input.modelConfigID, contentVersion, toolContent),
	}
	for i := range messages {
		messages[i].Compressed = true
	}
	// Replay rows stay uncompressed and sort after the boundary trio, so
	// the prompt window query includes them on the next generation pass.
	for _, row := range input.pendingUserMessages {
		messages = append(messages, chatstate.Message{
			Role:           database.ChatMessageRoleUser,
			Content:        row.Content,
			Visibility:     database.ChatMessageVisibilityModel,
			ModelConfigID:  uuid.NullUUID{UUID: input.modelConfigID, Valid: input.modelConfigID != uuid.Nil},
			ContentVersion: row.ContentVersion,
		})
	}
	return compactionMessagesForCommit{Messages: messages, HiddenCount: 1}, nil
}

type buildClearMessagesInput struct {
	modelConfigID  uuid.UUID
	toolCallID     string
	contentVersion int16
}

// buildClearMessages produces the manual context-clear boundary
// triplet, mirroring the compaction triplet shape: a hidden
// model-only user-role row (the boundary anchor the prompt query keys
// on), a user-visible synthetic chat_cleared tool call, and its tool
// result. The hidden row carries a short sentinel rather than empty
// content so the next prompt never sends an empty user message.
func buildClearMessages(input buildClearMessagesInput) ([]chatstate.Message, error) {
	contentVersion := input.contentVersion
	if contentVersion == 0 {
		contentVersion = chatprompt.CurrentContentVersion
	}

	sentinelContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText("Previous conversation context was cleared by the user."),
	})
	if err != nil {
		return nil, xerrors.Errorf("marshal clear sentinel: %w", err)
	}
	payload := json.RawMessage(`{"source":"manual"}`)
	assistantContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolCall(input.toolCallID, "chat_cleared", payload),
	})
	if err != nil {
		return nil, xerrors.Errorf("marshal clear tool call: %w", err)
	}
	toolContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolResult(input.toolCallID, "chat_cleared", payload, false, false),
	})
	if err != nil {
		return nil, xerrors.Errorf("marshal clear tool result: %w", err)
	}

	messages := []chatstate.Message{
		{
			Role:           database.ChatMessageRoleUser,
			Content:        sentinelContent,
			Visibility:     database.ChatMessageVisibilityModel,
			ModelConfigID:  uuid.NullUUID{UUID: input.modelConfigID, Valid: input.modelConfigID != uuid.Nil},
			ContentVersion: contentVersion,
		},
		baseMessage(database.ChatMessageRoleAssistant, database.ChatMessageVisibilityUser, input.modelConfigID, contentVersion, assistantContent),
		baseMessage(database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, input.modelConfigID, contentVersion, toolContent),
	}
	for i := range messages {
		messages[i].Compressed = true
	}
	return messages, nil
}

// hasClearableMessageAfter reports whether any active, uncompressed
// model-visible conversation message follows the boundary index.
// System prompts and user-only rows do not make a chat clearable.
func hasClearableMessageAfter(messages []database.ChatMessage, index int) bool {
	for i := index + 1; i < len(messages); i++ {
		msg := messages[i]
		if msg.Deleted || msg.Compressed {
			continue
		}
		if msg.Role == database.ChatMessageRoleSystem {
			continue
		}
		if msg.Visibility == database.ChatMessageVisibilityModel || msg.Visibility == database.ChatMessageVisibilityBoth {
			return true
		}
	}
	return false
}

// Hook model-context messages use the user role but must not reset
// per-turn guards.
func lastUserPromptIndex(messages []database.ChatMessage) int {
	index := -1
	for i, msg := range messages {
		if msg.Deleted || msg.Compressed {
			continue
		}
		if msg.Role == database.ChatMessageRoleUser && msg.Visibility != database.ChatMessageVisibilityModel {
			index = i
		}
	}
	return index
}

func currentTurnStartIndex(messages []database.ChatMessage) int {
	return lastUserPromptIndex(messages) + 1
}

func currentTurnStepCount(messages []database.ChatMessage) int {
	count := 0
	for _, msg := range messages[currentTurnStartIndex(messages):] {
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
	boundaryIndex := latestContextBoundaryIndex(messages)
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

// latestContextBoundaryIndex finds the latest compressed
// chat_summarized or chat_cleared boundary. Compaction and clear
// eligibility both stop here so neither reaches across the other.
func latestContextBoundaryIndex(messages []database.ChatMessage) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if isContextBoundaryMessage(messages[i]) {
			return i
		}
	}
	return -1
}

func isContextBoundaryMessage(msg database.ChatMessage) bool {
	if msg.Deleted || !msg.Compressed {
		return false
	}
	parts, err := chatprompt.ParseContent(msg)
	if err != nil {
		return false
	}
	for _, part := range parts {
		if (part.ToolName == "chat_summarized" || part.ToolName == "chat_cleared") &&
			(part.Type == codersdk.ChatMessagePartTypeToolCall || part.Type == codersdk.ChatMessagePartTypeToolResult) {
			return true
		}
	}
	return false
}

// pendingUserSegmentStart returns the index of the first row of the
// unanswered trailing user segment: the contiguous run of user-role
// rows after the last assistant or tool row. Returns len(promptRows)
// when there is no such segment. The segment is excluded from the
// summarizer's input and re-inserted verbatim after the compaction
// boundary instead of being summarized (CODAGT-737).
//
// The scan runs on persisted rows, not the converted prompt: assistant
// rows terminate the segment even when prompt conversion drops them,
// so an older user message can never be mistaken for pending.
func pendingUserSegmentStart(promptRows []database.ChatMessage) int {
	start := len(promptRows)
	for start > 0 {
		row := promptRows[start-1]
		if row.Deleted || row.Compressed || row.Role != database.ChatMessageRoleUser {
			break
		}
		start--
	}
	return start
}

func hasAssistantMessage(prompt []fantasy.Message) bool {
	for _, msg := range prompt {
		if msg.Role == fantasy.MessageRoleAssistant {
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
	for _, msg := range messages[currentTurnStartIndex(messages):] {
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
	// attemptRuntime is the interrupted attempt's billable model
	// invocation window: the span from the provider stream opening to
	// the interrupt closing its buffer episode. It is persisted as
	// runtime_ms on the first partial assistant message when the
	// attempt streamed model-generated assistant content.
	attemptRuntime time.Duration
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
	if input.attemptRuntime > 0 && state.modelStreamedAssistant {
		// Usage reporting sums runtime_ms across rows, so placing the
		// whole span on the first assistant message is sufficient.
		for i := range state.messages {
			if state.messages[i].Role != database.ChatMessageRoleAssistant {
				continue
			}
			state.messages[i].RuntimeMs = nullInt64IfNonZero(input.attemptRuntime.Milliseconds())
			break
		}
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
	// modelStreamedAssistant distinguishes streamed content from tool
	// attachment parts, which must not carry model runtime.
	modelStreamedAssistant bool
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
	if part.Type != codersdk.ChatMessagePartTypeFile {
		s.modelStreamedAssistant = true
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
