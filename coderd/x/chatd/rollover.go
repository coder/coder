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
	rolloverToolName = "chat_context_rollover"
	// finalHandoffReserveTokens leaves room below the usable limit for the
	// final prompt and the agent's checkpoint before the forced rollover.
	finalHandoffReserveTokens  = int64(16_000)
	minHandoffThresholdPercent = int32(10)
	firstContextWindowID       = "w:0"
	rolloverRequestedBySystem  = "system"
	rolloverRequestedByModel   = "model"
)

// contextWindowBudget holds the token budget points of one context
// window under the token budget compaction strategy. A zero
// contextLimit means the window size is unknown and no automatic stage
// fires.
type contextWindowBudget struct {
	contextLimit     int64
	usableLimit      int64
	finalPoint       int64
	handoffPoint     int64
	thresholdPercent int32
}

func newContextWindowBudget(contextLimit, outputCap int64, thresholdPercent int32) contextWindowBudget {
	budget := contextWindowBudget{thresholdPercent: thresholdPercent}
	if contextLimit <= 0 {
		return budget
	}
	budget.contextLimit = contextLimit
	budget.usableLimit = contextLimit - min(outputCap, contextLimit/4)
	budget.finalPoint = max(budget.usableLimit-finalHandoffReserveTokens, (budget.usableLimit+1)/2)
	budget.handoffPoint = min(contextLimit*int64(max(thresholdPercent, minHandoffThresholdPercent))/100, budget.finalPoint)
	return budget
}

type rolloverAction string

const (
	rolloverActionNone     rolloverAction = ""
	rolloverActionHandoff  rolloverAction = "handoff"
	rolloverActionFinal    rolloverAction = "final"
	rolloverActionRollover rolloverAction = "rollover"
)

type rolloverReason string

const (
	rolloverReasonModel rolloverReason = "model"
	rolloverReasonLimit rolloverReason = "limit"
)

type rolloverDecision struct {
	action rolloverAction
	reason rolloverReason
}

// rolloverStageState is derived from the rows of the current context
// window, so claims and receipts survive restarts and reset at every
// boundary.
type rolloverStageState struct {
	handoffClaimed bool
	finalClaimed   bool
	// receipt is an unconsumed successful new_context result.
	receipt           bool
	newContextOffered bool
}

func decideRollover(contextTokens int64, budget contextWindowBudget, state rolloverStageState) rolloverDecision {
	switch {
	case budget.thresholdPercent >= chatloop.CompactionDisabledThresholdPercent:
		return rolloverDecision{action: rolloverActionNone}
	case state.receipt:
		return rolloverDecision{action: rolloverActionRollover, reason: rolloverReasonModel}
	case budget.contextLimit <= 0 || contextTokens <= 0:
		return rolloverDecision{action: rolloverActionNone}
	case contextTokens >= budget.usableLimit:
		return rolloverDecision{action: rolloverActionRollover, reason: rolloverReasonLimit}
	case !state.handoffClaimed && contextTokens >= budget.handoffPoint:
		return rolloverDecision{action: rolloverActionHandoff}
	case state.handoffClaimed && !state.finalClaimed && state.newContextOffered && contextTokens >= budget.finalPoint:
		return rolloverDecision{action: rolloverActionFinal}
	default:
		return rolloverDecision{action: rolloverActionNone}
	}
}

// windowContextTokens returns the provider-reported context usage of the
// newest assistant step in the current context window, or zero when the
// window has none. Usage from before the latest boundary must not count:
// it would roll the fresh window over again at once.
func windowContextTokens(messages []database.ChatMessage) int64 {
	boundary := latestContextBoundaryIndex(messages)
	for i := len(messages) - 1; i > boundary; i-- {
		msg := messages[i]
		if msg.Deleted || msg.Compressed || msg.Role != database.ChatMessageRoleAssistant {
			continue
		}
		if tokens := contextTokensFromUsage(usageFromMessage(msg)); tokens > 0 {
			return tokens
		}
	}
	return 0
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
// and result. The call omits the checkpoint.
type contextRolloverPayload struct {
	Reason           rolloverReason `json:"reason"`
	RequestedBy      string         `json:"requested_by"`
	PreviousWindowID string         `json:"previous_window_id"`
	ContextTokens    int64          `json:"context_tokens"`
	BudgetTokens     int64          `json:"budget_tokens"`
	Checkpoint       string         `json:"checkpoint,omitempty"`
}

type buildRolloverMessagesInput struct {
	modelConfigID uuid.UUID
	toolCallID    string
	payload       contextRolloverPayload
	// requestItemID is the message ID of the user request in progress,
	// or zero when there is none.
	requestItemID       int64
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
	callPayload := input.payload
	callPayload.Checkpoint = ""
	args, err := json.Marshal(callPayload)
	if err != nil {
		return nil, xerrors.Errorf("marshal rollover args: %w", err)
	}
	result, err := json.Marshal(input.payload)
	if err != nil {
		return nil, xerrors.Errorf("marshal rollover result: %w", err)
	}
	callContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolCall(input.toolCallID, rolloverToolName, args),
	})
	if err != nil {
		return nil, xerrors.Errorf("marshal rollover tool call: %w", err)
	}
	resultContent, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageToolResult(input.toolCallID, rolloverToolName, result, false, false),
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
	payload, requestItemID := input.payload, input.requestItemID
	midTurn := len(input.pendingUserMessages) == 0
	lines := []string{fmt.Sprintf("A context window rollover started a fresh context window. Previous window: %s.", payload.PreviousWindowID)}
	if payload.Reason == rolloverReasonModel {
		lines = append(lines, "You requested this fresh window with new_context; continue the task.")
	} else {
		if midTurn {
			lines = append(lines, "Your previous turn was interrupted by a context rollover; continue the task.")
		}
		lines = append(lines, "The window reached its usable limit.")
	}
	recovery := "Completed tool results remain in the previous window: retrieve them with chat_history rather than re-executing their side effects."
	if requestItemID > 0 {
		recovery += fmt.Sprintf(" The request in progress is item %d.", requestItemID)
	}
	lines = append(lines, recovery)
	switch checkpoint := strings.TrimSpace(payload.Checkpoint); {
	case checkpoint != "":
		lines = append(lines, "<checkpoint>\n"+checkpoint+"\n</checkpoint>")
	case requestItemID > 0:
		lines = append(lines, fmt.Sprintf("No checkpoint was saved; recover the task with chat_history, starting with item %d.", requestItemID))
	default:
		lines = append(lines, "No checkpoint was saved; recover the task with chat_history.")
	}
	return strings.Join(lines, "\n")
}

// latestRequestItemID returns the message ID of the latest user request.
func latestRequestItemID(messages []database.ChatMessage) int64 {
	if index := lastUserPromptIndex(messages); index >= 0 {
		return messages[index].ID
	}
	return 0
}
