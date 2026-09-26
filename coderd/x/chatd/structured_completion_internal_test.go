package chatd

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstructured"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestDecideStructuredGenerationAction(t *testing.T) {
	t.Parallel()
	requestID := uuid.New()
	part := func(p codersdk.ChatMessagePart, err error) codersdk.ChatMessagePart {
		require.NoError(t, err)
		return p
	}
	request := part(chatstructured.EncodeRequestPart(chatstructured.Request{RequestID: requestID, Name: "report", Schema: json.RawMessage(`{}`)}))
	rejection := part(chatstructured.EncodeControlPart(chatstructured.Control{RequestID: requestID, Kind: chatstructured.ControlRejection}))
	candidate := part(chatstructured.EncodeControlPart(chatstructured.Control{RequestID: requestID, Kind: chatstructured.ControlCandidate, Value: json.RawMessage(`{"a":1}`)}))
	fin := chatstructured.FinalizerToolName
	row := func(role database.ChatMessageRole, parts ...codersdk.ChatMessagePart) database.ChatMessage {
		return dbMessage(t, 0, role, false, parts...)
	}
	call := func(id, name string) database.ChatMessage {
		return row(database.ChatMessageRoleAssistant, codersdk.ChatMessageToolCall(id, name, json.RawMessage(`{}`)))
	}
	result := func(id, name string, extra ...codersdk.ChatMessagePart) database.ChatMessage {
		return row(database.ChatMessageRoleTool, append([]codersdk.ChatMessagePart{codersdk.ChatMessageToolResult(id, name, json.RawMessage(`{}`), false, false)}, extra...)...)
	}
	req := row(database.ChatMessageRoleUser, codersdk.ChatMessageText("q"), request)
	text := row(database.ChatMessageRoleAssistant, codersdk.ChatMessageText("a"))
	rejectedText := row(database.ChatMessageRoleAssistant, codersdk.ChatMessageText("a"), rejection)
	hookContext := row(database.ChatMessageRoleUser, codersdk.ChatMessageText("continue"), part(chatstructured.EncodeControlPart(chatstructured.Control{RequestID: requestID, Kind: chatstructured.ControlInvalidation})))
	hookContext.Visibility = database.ChatMessageVisibilityModel
	badSubmission := []database.ChatMessage{call("f", fin), result("f", fin, rejection)}
	for _, tt := range []struct {
		name     string
		history  []database.ChatMessage
		maxSteps int
		wantKind generationActionKind
		wantCode codersdk.ChatStructuredOutputErrorCode
		success  bool
	}{
		{name: "Ordinary", history: []database.ChatMessage{row(database.ChatMessageRoleUser, codersdk.ChatMessageText("q")), text}, wantKind: generationActionFinishTurn},
		{name: "RepairText", history: []database.ChatMessage{req, rejectedText}, wantKind: generationActionGenerateAssistant},
		{name: "UnrepairedText", history: []database.ChatMessage{req, text}, wantKind: generationActionFinishTurn, wantCode: codersdk.ChatStructuredOutputErrorCodeNotProduced},
		{name: "TextBudget", history: []database.ChatMessage{req, rejectedText, rejectedText, rejectedText}, wantKind: generationActionFinishTurn, wantCode: codersdk.ChatStructuredOutputErrorCodeNotProduced},
		{name: "SubmissionBudget", history: append(append(append([]database.ChatMessage{req}, badSubmission...), badSubmission...), badSubmission...), wantKind: generationActionFinishTurn, wantCode: codersdk.ChatStructuredOutputErrorCodeValidationExhausted},
		{name: "Success", history: []database.ChatMessage{req, call("f", fin), result("f", fin, candidate), text}, wantKind: generationActionFinishTurn, success: true},
		{name: "StepBudget", history: []database.ChatMessage{req, call("x", "execute"), result("x", "execute")}, maxSteps: 1, wantKind: generationActionFinishTurn, wantCode: codersdk.ChatStructuredOutputErrorCodeNotProduced},
		{name: "CandidateKeepsWorking", history: []database.ChatMessage{req, call("f", fin), result("f", fin, candidate)}, wantKind: generationActionGenerateAssistant},
		{name: "StepBudgetCandidate", history: []database.ChatMessage{req, call("f", fin), result("f", fin, candidate), call("x", "execute"), result("x", "execute")}, maxSteps: 2, wantKind: generationActionFinishTurn, success: true},
		// After a restart no nudge forces a step: the invalidated candidate
		// cannot close the turn.
		{name: "InvalidatedCandidate", history: []database.ChatMessage{req, call("f", fin), result("f", fin, candidate), text, hookContext}, wantKind: generationActionFinishTurn, wantCode: codersdk.ChatStructuredOutputErrorCodeNotProduced},
		{name: "RepairStopAfter", history: []database.ChatMessage{req, call("e", "exit"), result("e", "exit", rejection)}, wantKind: generationActionGenerateAssistant},
		{name: "UnrepairedStopAfter", history: []database.ChatMessage{req, call("e", "exit"), result("e", "exit")}, wantKind: generationActionFinishTurn, wantCode: codersdk.ChatStructuredOutputErrorCodeNotProduced},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			history := make([]database.ChatMessage, len(tt.history))
			for i, msg := range tt.history {
				msg.ID = int64(i + 1)
				history[i] = msg
			}
			state, _ := openStructuredRequest(context.Background(), testutil.Logger(t), uuid.New(), history)
			// Generation decides from user-visible rows and the state of all rows.
			visible := slices.DeleteFunc(slices.Clone(history), func(msg database.ChatMessage) bool { return msg.Visibility == database.ChatMessageVisibilityModel })
			decision, err := decideGenerationAction(generationDecisionInput{
				messages: visible, maxSteps: tt.maxSteps, stopAfterTools: map[string]struct{}{"exit": {}}, structured: state,
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantKind, decision.kind)
			switch {
			case tt.success:
				require.NotNil(t, decision.structured)
				require.Equal(t, codersdk.ChatStructuredOutputStatusSucceeded, decision.structured.outcome.Status)
				require.JSONEq(t, `{"a":1}`, string(decision.structured.outcome.Value))
			case tt.wantCode != "":
				require.NotNil(t, decision.structured)
				require.Equal(t, tt.wantCode, decision.structured.outcome.Error.Code)
			default:
				require.Nil(t, decision.structured)
			}
		})
	}
}

// The finish commit re-reads the request under the chat lock, so a success
// decided before a stop hook await never commits a candidate that changed.
func TestStructuredFinishRereadsCandidate(t *testing.T) {
	t.Parallel()
	requestID := uuid.New()
	encode := func(p codersdk.ChatMessagePart, err error) codersdk.ChatMessagePart {
		require.NoError(t, err)
		return p
	}
	control := func(kind chatstructured.ControlKind, value json.RawMessage) codersdk.ChatMessagePart {
		return encode(chatstructured.EncodeControlPart(chatstructured.Control{RequestID: requestID, Kind: kind, Value: value}))
	}
	success := codersdk.ChatStructuredOutput{RequestID: requestID, Status: codersdk.ChatStructuredOutputStatusSucceeded, Value: json.RawMessage(`{"a":1}`)}
	invalidated := dbMessage(t, 0, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("continue"), control(chatstructured.ControlInvalidation, nil))
	invalidated.Visibility = database.ChatMessageVisibilityModel
	receiptParts, err := chatstructured.ReceiptParts(canceledStructuredOutput(requestID, codersdk.ChatStructuredOutputErrorCodeQueueDeleted, queueDeletedStructuredOutputMessage))
	require.NoError(t, err)
	closed := dbMessage(t, 0, database.ChatMessageRoleAssistant, false, receiptParts...)
	closed.Visibility = database.ChatMessageVisibilityUser
	for _, tt := range []struct {
		name  string
		after []database.ChatMessage
		want  codersdk.ChatStructuredOutputStatus // empty for no receipt
	}{
		{name: "Unchanged", want: codersdk.ChatStructuredOutputStatusSucceeded},
		{name: "Invalidated", after: []database.ChatMessage{invalidated}, want: codersdk.ChatStructuredOutputStatusFailed},
		{name: "Closed", after: []database.ChatMessage{closed}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db, _ := dbtestutil.NewDB(t)
			ctx := testutil.Context(t, testutil.WaitLong)
			user := dbgen.User(t, db, database.User{})
			org := dbgen.Organization(t, db, database.Organization{})
			dbgen.ChatProvider(t, db, database.ChatProvider{})
			model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{IsDefault: true})
			chat := dbgen.Chat(t, db, database.Chat{OwnerID: user.ID, OrganizationID: org.ID, LastModelConfigID: model.ID})
			var requestRowID int64
			for i, msg := range append([]database.ChatMessage{
				dbMessage(t, 0, database.ChatMessageRoleUser, false, codersdk.ChatMessageText("q"), encode(chatstructured.EncodeRequestPart(chatstructured.Request{RequestID: requestID, Name: "report", Schema: json.RawMessage(`{}`)}))),
				dbMessage(t, 0, database.ChatMessageRoleTool, false, codersdk.ChatMessageToolResult("f", chatstructured.FinalizerToolName, json.RawMessage(`{}`), false, false), control(chatstructured.ControlCandidate, success.Value)),
			}, tt.after...) {
				msg.ChatID = chat.ID
				if inserted := dbgen.ChatMessage(t, db, msg); i == 0 {
					requestRowID = inserted.ID
				}
			}
			messages, err := structuredFinishMessages(ctx, testutil.Logger(t), db, chat.ID, &structuredTerminal{requestRowID: requestRowID, outcome: success})
			require.NoError(t, err)
			if tt.want == "" {
				require.Empty(t, messages)
				return
			}
			require.Len(t, messages, 1)
			got, err := chatstate.ReceiptRowOutcome(database.ChatMessage{Role: messages[0].Role, Visibility: messages[0].Visibility, Content: messages[0].Content, ContentVersion: messages[0].ContentVersion})
			require.NoError(t, err)
			require.Equal(t, tt.want, got.Status)
		})
	}
}
