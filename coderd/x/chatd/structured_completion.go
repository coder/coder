package chatd

import (
	"bytes"
	"context"
	"slices"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/codersdk"
)

// structuredRepairBudget is the number of rejected batches that ends a turn
// with an open structured output request: the first one and two repairs.
const structuredRepairBudget = 3

// structuredCorrectiveText is the model-only reminder committed after a step
// that ended without a candidate. It never quotes values.
const structuredCorrectiveText = "This request needs a structured final answer. Call " + chatstructured.FinalizerToolName + " with the output; a text answer or a stopping tool does not complete it."

// structuredFailureMessages are the fixed messages of failed receipts.
var structuredFailureMessages = map[codersdk.ChatStructuredOutputErrorCode]string{
	codersdk.ChatStructuredOutputErrorCodeNotProduced:         "The model ended the turn without calling " + chatstructured.FinalizerToolName + " with an output.",
	codersdk.ChatStructuredOutputErrorCodeValidationExhausted: "The model's structured output was rejected too many times.",
	codersdk.ChatStructuredOutputErrorCodeGenerationFailed:    "The turn failed before it produced the structured output.",
}

// structuredTerminal is the receipt that closes an open structured output
// request when its turn finishes.
type structuredTerminal struct {
	requestRowID int64
	outcome      codersdk.ChatStructuredOutput
}

func structuredFailure(state chatstructured.ActiveRequestState, code codersdk.ChatStructuredOutputErrorCode) *structuredTerminal {
	return &structuredTerminal{requestRowID: state.RequestRowID, outcome: codersdk.ChatStructuredOutput{
		RequestID: state.Request.RequestID, Status: codersdk.ChatStructuredOutputStatusFailed,
		Error: &codersdk.ChatStructuredOutputError{Code: code, Message: structuredFailureMessages[code]},
	}}
}

// decideStructuredTurn decides from history whether a turn with an open
// request ends, and with which receipt, or nil to keep generating. A current
// candidate succeeds whenever the turn would end; without one the turn fails
// at the step or repair budget, or when its latest step would end it
// unrepaired, which endStep forces. The failure is validation_exhausted when
// any rejection answered a finalizer call.
func decideStructuredTurn(input generationDecisionInput, endStep bool) (*structuredTerminal, error) {
	stopAfter, repairing, submission, err := structuredStepFacts(input.messages, input.structured.RequestRowID, input.stopAfterTools)
	if err != nil {
		return nil, err
	}
	complete, err := currentHistoryComplete(input.messages)
	if err != nil {
		return nil, err
	}
	state := input.structured
	finishing := stopAfter || complete || endStep
	exhausted := input.maxSteps > 0 && currentTurnStepCount(input.messages) >= input.maxSteps
	switch {
	case state.Candidate != nil && (finishing || exhausted):
		return &structuredTerminal{requestRowID: state.RequestRowID, outcome: codersdk.ChatStructuredOutput{
			RequestID: state.Request.RequestID, Status: codersdk.ChatStructuredOutputStatusSucceeded, Value: state.Candidate,
		}}, nil
	case state.Candidate != nil, state.Rejections < structuredRepairBudget && !exhausted && (!finishing || repairing && !endStep):
		return nil, nil //nolint:nilnil // A nil receipt means the turn keeps generating.
	case submission:
		return structuredFailure(state, codersdk.ChatStructuredOutputErrorCodeValidationExhausted), nil
	}
	return structuredFailure(state, codersdk.ChatStructuredOutputErrorCodeNotProduced), nil
}

// structuredStepFacts reads the rows after the request row: whether the
// latest step ran a stop-after tool or carries a rejection, and whether any
// rejection answered a finalizer submission. A rejection records its cause
// by the row it rides on: screening and the executor put it on a row holding
// a finalizer call or result, a repair on a text or stop-after row.
func structuredStepFacts(messages []database.ChatMessage, requestRowID int64, stopAfterTools map[string]struct{}) (stopAfter, rejected, submission bool, err error) {
	step := lastMessageIndex(messages, func(msg database.ChatMessage) bool {
		return msg.Role == database.ChatMessageRoleAssistant && !chatstate.IsReceiptRow(msg)
	})
	for i, msg := range messages {
		if msg.ID <= requestRowID {
			continue
		}
		parts, err := chatprompt.ParseContent(msg)
		if err != nil {
			return false, false, false, xerrors.Errorf("parse message: %w", err)
		}
		latest := step >= 0 && i >= step && !msg.Deleted && !msg.Compressed
		finalizer, rejection := false, false
		for _, part := range parts {
			switch part.Type {
			case codersdk.ChatMessagePartTypeToolCall, codersdk.ChatMessagePartTypeToolResult:
				finalizer = finalizer || part.ToolName == chatstructured.FinalizerToolName
				if _, ok := stopAfterTools[part.ToolName]; ok && latest && part.Type == codersdk.ChatMessagePartTypeToolResult && !part.IsError {
					stopAfter = true
				}
			case codersdk.ChatMessagePartTypeStructuredOutputControl:
				control, err := chatstructured.DecodeControlPart(part)
				rejection = rejection || (err == nil && control.Kind == chatstructured.ControlRejection)
			}
		}
		submission = submission || (rejection && finalizer)
		rejected = rejected || (rejection && latest)
	}
	return stopAfter, rejected, submission, nil
}

// structuredRepair returns the rejection for a step that ended without a
// candidate and, unless it spends the repair budget, the model-only
// corrective row to commit after it. Without an open request, or with a
// candidate, it returns nothing.
func structuredRepair(prepared generationPrepared) ([]codersdk.ChatMessagePart, []chatstate.Message, error) {
	state := prepared.Structured
	if !state.Active || state.Candidate != nil {
		return nil, nil, nil
	}
	control, err := chatstructured.EncodeControlPart(chatstructured.Control{RequestID: state.Request.RequestID, Kind: chatstructured.ControlRejection})
	if err != nil {
		return nil, nil, xerrors.Errorf("encode structured output rejection: %w", err)
	}
	if state.Rejections+1 >= structuredRepairBudget {
		return []codersdk.ChatMessagePart{control}, nil, nil
	}
	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(structuredCorrectiveText)})
	if err != nil {
		return nil, nil, xerrors.Errorf("marshal structured output corrective: %w", err)
	}
	return []codersdk.ChatMessagePart{control}, []chatstate.Message{baseMessage(database.ChatMessageRoleUser, database.ChatMessageVisibilityModel, prepared.ModelConfigID, chatprompt.CurrentContentVersion, content)}, nil
}

// stopAfterResultID returns the call ID of the first successful stop-after
// tool result in content, or "".
func stopAfterResultID(content []fantasy.Content, stopAfterTools map[string]struct{}) string {
	for _, block := range content {
		result, ok := fantasy.AsContentType[fantasy.ToolResultContent](block)
		if _, stops := stopAfterTools[result.ToolName]; ok && stops {
			if _, failed := result.Result.(fantasy.ToolResultOutputContentError); !failed {
				return result.ToolCallID
			}
		}
	}
	return ""
}

// structuredFinishMessages returns the receipt FinishTurn commits for t,
// re-reading the request under the chat lock: a stop hook await, or any
// other gap since the decision, may have closed the request or changed its
// candidate. A success never commits a candidate that is no longer current;
// the current candidate succeeds instead, and without one the request fails
// by the decision's rules. The executor only records candidates whose
// receipt encodes.
func structuredFinishMessages(ctx context.Context, logger slog.Logger, store database.Store, chatID uuid.UUID, t *structuredTerminal) ([]chatstate.Message, error) {
	if t == nil {
		return nil, nil
	}
	history, err := store.GetChatMessagesAllVisibilitiesByChatID(ctx, chatID)
	if err != nil {
		return nil, xerrors.Errorf("load structured output history: %w", err)
	}
	state, open := openStructuredRequest(ctx, logger, chatID, history)
	if !open || state.Request.RequestID != t.outcome.RequestID {
		return nil, nil
	}
	out := t.outcome
	if out.Status == codersdk.ChatStructuredOutputStatusSucceeded && !bytes.Equal(state.Candidate, out.Value) {
		out.Value = state.Candidate
		if state.Candidate == nil {
			_, _, submission, err := structuredStepFacts(history, state.RequestRowID, nil)
			if err != nil {
				return nil, err
			}
			code := codersdk.ChatStructuredOutputErrorCodeNotProduced
			if submission {
				code = codersdk.ChatStructuredOutputErrorCodeValidationExhausted
			}
			out = structuredFailure(state, code).outcome
		}
	}
	return structuredReceiptMessages(ctx, store, chatID, t.requestRowID, out)
}

// structuredInvalidation returns the stop hook messages of a continuing
// turn with one invalidation control appended to the hook's model-only
// context row when the open request has a candidate, so the resumed turn
// must finalize again, also after a restart. No client sees the control or
// a new row. Run it inside the ChatMachine.Update that commits the messages;
// visible is the user-visible history read before the hook.
func structuredInvalidation(ctx context.Context, logger slog.Logger, store database.Store, chatID uuid.UUID, visible []database.ChatMessage, messages []chatstate.Message) ([]chatstate.Message, error) {
	i := slices.IndexFunc(messages, func(msg chatstate.Message) bool { return msg.Visibility == database.ChatMessageVisibilityModel })
	if i < 0 {
		return messages, nil
	}
	history, err := structuredStateHistory(ctx, store, chatID, visible)
	if err != nil {
		return nil, err
	}
	state, open := openStructuredRequest(ctx, logger, chatID, history)
	if !open || state.Candidate == nil {
		return messages, nil
	}
	row := messages[i]
	parts, err := chatprompt.ParseContent(database.ChatMessage{Role: row.Role, Visibility: row.Visibility, Content: row.Content, ContentVersion: row.ContentVersion})
	if err != nil {
		return nil, xerrors.Errorf("parse stop hook context: %w", err)
	}
	control, err := chatstructured.EncodeControlPart(chatstructured.Control{RequestID: state.Request.RequestID, Kind: chatstructured.ControlInvalidation})
	if err != nil {
		return nil, xerrors.Errorf("encode structured output invalidation: %w", err)
	}
	if row.Content, err = chatprompt.MarshalParts(append(parts, control)); err != nil {
		return nil, xerrors.Errorf("marshal stop hook context: %w", err)
	}
	messages = slices.Clone(messages)
	messages[i] = row
	return messages, nil
}

// generationFailedReceipts returns the failed generation_failed receipt that
// closes the open structured output request when a turn ends in an error.
func generationFailedReceipts(ctx context.Context, logger slog.Logger, store database.Store, chatID uuid.UUID) ([]chatstate.Message, error) {
	history, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: chatID})
	if err != nil {
		return nil, xerrors.Errorf("load history for structured output receipt: %w", err)
	}
	return activeRequestReceipt(ctx, logger, store, chatID, history,
		structuredFailure(chatstructured.ActiveRequestState{}, codersdk.ChatStructuredOutputErrorCodeGenerationFailed).outcome)
}
