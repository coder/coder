package chatd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
)

const (
	rolloverToolName          = "chat_context_rollover"
	firstContextWindowID      = "w:0"
	rolloverRequestedBySystem = "system"
	// rolloverLeadInPrefix starts the first line of a rollover anchor,
	// which ends with the previous window ID.
	rolloverLeadInPrefix = "A context window rollover started a fresh context window. Previous window: "
)

// contextWindowBudget holds the token budget points of one context
// window under the token budget compaction strategy. A zero
// contextLimit means the window size is unknown and no automatic stage
// fires.
type contextWindowBudget struct {
	contextLimit     int64
	usableLimit      int64
	thresholdPercent int32
}

func newContextWindowBudget(contextLimit, outputCap int64, thresholdPercent int32) contextWindowBudget {
	budget := contextWindowBudget{thresholdPercent: thresholdPercent}
	if contextLimit <= 0 {
		return budget
	}
	budget.contextLimit = contextLimit
	budget.usableLimit = contextLimit - min(outputCap, contextLimit/4)
	return budget
}

type rolloverAction string

const (
	rolloverActionNone     rolloverAction = ""
	rolloverActionRollover rolloverAction = "rollover"
)

type rolloverReason string

const (
	rolloverReasonLimit rolloverReason = "limit"
)

type rolloverDecision struct {
	action rolloverAction
	reason rolloverReason
}

func decideRollover(contextTokens int64, budget contextWindowBudget) rolloverDecision {
	switch {
	case budget.thresholdPercent >= chatloop.CompactionDisabledThresholdPercent:
		return rolloverDecision{action: rolloverActionNone}
	case budget.contextLimit <= 0 || contextTokens <= 0:
		return rolloverDecision{action: rolloverActionNone}
	case contextTokens >= budget.usableLimit:
		return rolloverDecision{action: rolloverActionRollover, reason: rolloverReasonLimit}
	default:
		return rolloverDecision{action: rolloverActionNone}
	}
}

// contextWindowID names the current context window: w:0 before any
// boundary, then w:<anchor message ID> of the latest boundary of any
// kind. It reads prompt rows, whose only compressed row is that anchor.
func contextWindowID(promptRows []database.ChatMessage) string {
	for i := len(promptRows) - 1; i >= 0; i-- {
		msg := promptRows[i]
		if !msg.Deleted && msg.Compressed && msg.Role == database.ChatMessageRoleUser && msg.Visibility == database.ChatMessageVisibilityModel {
			return fmt.Sprintf("w:%d", msg.ID)
		}
	}
	return firstContextWindowID
}

// contextRolloverPayload is the chat_context_rollover tool call input
// and result.
type contextRolloverPayload struct {
	Reason           rolloverReason `json:"reason"`
	RequestedBy      string         `json:"requested_by"`
	PreviousWindowID string         `json:"previous_window_id"`
	ContextTokens    int64          `json:"context_tokens"`
	BudgetTokens     int64          `json:"budget_tokens"`
}

type buildRolloverMessagesInput struct {
	modelConfigID       uuid.UUID
	toolCallID          string
	payload             contextRolloverPayload
	pendingUserMessages []database.ChatMessage
}

// buildRolloverMessages produces the rollover boundary triplet in the
// compaction triplet's shape, with the lead-in as the anchor, followed by
// model-only replays of the pending user rows.
func buildRolloverMessages(input buildRolloverMessagesInput) ([]chatstate.Message, error) {
	contentVersion := chatprompt.CurrentContentVersion
	leadIn := rolloverLeadIn(input)
	anchorContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(leadIn)})
	if err != nil {
		return nil, xerrors.Errorf("marshal rollover lead-in: %w", err)
	}
	payload, err := json.Marshal(input.payload)
	if err != nil {
		return nil, xerrors.Errorf("marshal rollover payload: %w", err)
	}
	callContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolCall(input.toolCallID, rolloverToolName, payload),
	})
	if err != nil {
		return nil, xerrors.Errorf("marshal rollover tool call: %w", err)
	}
	resultContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolResult(input.toolCallID, rolloverToolName, payload, false, false),
	})
	if err != nil {
		return nil, xerrors.Errorf("marshal rollover tool result: %w", err)
	}
	messages := []chatstate.Message{
		baseMessage(database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, input.modelConfigID, contentVersion, anchorContent),
		baseMessage(database.ChatMessageRoleAssistant, database.ChatMessageVisibilityUser, input.modelConfigID, contentVersion, callContent),
		baseMessage(database.ChatMessageRoleTool, database.ChatMessageVisibilityBoth, input.modelConfigID, contentVersion, resultContent),
	}
	for i := range messages {
		messages[i].Compressed = true
	}
	for _, row := range input.pendingUserMessages {
		replay := baseMessage(database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, input.modelConfigID, row.ContentVersion, row.Content)
		messages = append(messages, replay)
	}
	return messages, nil
}

// rolloverLeadIn is the model-visible anchor of a fresh context window.
// Without pending user rows the rollover interrupted work in progress
// rather than starting before a new user message is answered.
func rolloverLeadIn(input buildRolloverMessagesInput) string {
	payload := input.payload
	midTurn := len(input.pendingUserMessages) == 0
	lines := []string{rolloverLeadInPrefix + payload.PreviousWindowID + "."}
	if midTurn {
		lines = append(lines, "Your previous turn was interrupted by a context rollover; continue the task.")
	}
	lines = append(lines, "The window reached its usable limit.")
	return strings.Join(lines, "\n")
}
