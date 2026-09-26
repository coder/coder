package chatd

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
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
			decision, err := decideGenerationAction(generationDecisionInput{
				messages: history, maxSteps: tt.maxSteps, stopAfterTools: map[string]struct{}{"exit": {}}, structured: state,
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
