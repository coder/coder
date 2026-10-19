package chatstructured

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func requestPart(t *testing.T, id uuid.UUID) codersdk.ChatMessagePart {
	t.Helper()
	part, err := EncodeRequestPart(Request{RequestID: id, Name: "report", Schema: json.RawMessage(`{"type":"object"}`)})
	require.NoError(t, err)
	return part
}

func controlPart(t *testing.T, id uuid.UUID, kind ControlKind, value string) codersdk.ChatMessagePart {
	t.Helper()
	part, err := EncodeControlPart(Control{RequestID: id, Kind: kind, Value: rawOrNil(value)})
	require.NoError(t, err)
	return part
}

func outcomePart(t *testing.T, id uuid.UUID) codersdk.ChatMessagePart {
	t.Helper()
	part, err := EncodeOutcomePart(codersdk.ChatStructuredOutput{RequestID: id, Status: codersdk.ChatStructuredOutputStatusSucceeded, Value: json.RawMessage(`{"a":1}`)})
	require.NoError(t, err)
	return part
}

func rawOrNil(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	return json.RawMessage(s)
}

func TestActiveRequest(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	const user, assistant, system, tool = codersdk.ChatMessageRoleUser, codersdk.ChatMessageRoleAssistant, codersdk.ChatMessageRoleSystem, codersdk.ChatMessageRoleTool
	row := func(id int64, role codersdk.ChatMessageRole, vis Visibility, parts ...codersdk.ChatMessagePart) Row {
		return Row{ID: id, Role: role, Visibility: vis, Parts: parts}
	}
	text := codersdk.ChatMessageText
	notice := codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeHookNotice, Text: "n"}
	req := row(1, user, VisibilityBoth, text("go"), requestPart(t, a))
	// Compaction stores a model-only summary, a user-only display call, its
	// result, and model-only copies of pending user rows.
	compaction := []Row{
		row(10, user, VisibilityModel, text("summary")), row(11, assistant, VisibilityUser, codersdk.ChatMessageToolCall("c", "chat_summarized", nil)),
		row(12, tool, VisibilityBoth, codersdk.ChatMessageToolResult("c", "chat_summarized", nil, false, false)), row(13, user, VisibilityModel, req.Parts...),
	}
	step := func(id int64, parts ...codersdk.ChatMessagePart) Row {
		return row(id, assistant, VisibilityBoth, append([]codersdk.ChatMessagePart{text("x")}, parts...)...)
	}
	for _, tt := range []struct {
		name        string
		rows        []Row
		want        ActiveRequestState
		wantCorrupt bool
	}{
		{name: "Empty"},
		{name: "Ordinary", rows: []Row{row(1, user, VisibilityBoth, text("hi")), step(2)}},
		{name: "Request", rows: []Row{req}, want: ActiveRequestState{Active: true, RequestRowID: 1}},
		{name: "PlainTextSuccessor", rows: []Row{req, step(2), row(3, user, VisibilityBoth, text("next"))}},
		{name: "ForeignOutcomeSkipped", rows: []Row{req, row(2, assistant, VisibilityBoth, outcomePart(t, b)), step(3)}, want: ActiveRequestState{Active: true, RequestRowID: 1, GenerationSteps: 1}},
		// A receipt row in user role is not a new user turn.
		{name: "OutcomeRowNotUserTurn", rows: []Row{req, row(2, user, VisibilityBoth, text("done"), outcomePart(t, b))}, want: ActiveRequestState{Active: true, RequestRowID: 1}},
		{
			name: "CompactionAndHooks",
			rows: append([]Row{req, step(2), row(3, system, VisibilityUser, notice), row(4, user, VisibilityBoth, notice), row(5, user, VisibilityModel, text("hook"))}, compaction...),
			want: ActiveRequestState{Active: true, RequestRowID: 1, GenerationSteps: 1},
		},
		{
			name: "Controls",
			rows: []Row{
				req, step(2, controlPart(t, a, ControlCandidate, `1`)), row(3, assistant, VisibilityBoth, controlPart(t, a, ControlRejection, "")),
				step(4, controlPart(t, a, ControlCandidate, `2`)), row(5, assistant, VisibilityBoth, controlPart(t, a, ControlInvalidation, "")),
				step(6, controlPart(t, a, ControlCandidate, `null`)),
			},
			want: ActiveRequestState{Active: true, RequestRowID: 1, Rejections: 1, Candidate: json.RawMessage(`null`), GenerationSteps: 3},
		},
		{
			name: "Invalidated",
			rows: []Row{req, step(2, controlPart(t, a, ControlCandidate, `1`)), row(3, assistant, VisibilityBoth, controlPart(t, a, ControlInvalidation, ""))},
			want: ActiveRequestState{Active: true, RequestRowID: 1, GenerationSteps: 1, Invalidated: true},
		},
		{
			name: "InvalidationAnswered",
			rows: []Row{req, step(2, controlPart(t, a, ControlCandidate, `1`)), row(3, user, VisibilityModel, controlPart(t, a, ControlInvalidation, "")), step(4)},
			want: ActiveRequestState{Active: true, RequestRowID: 1, GenerationSteps: 2},
		},
		{
			name: "InvalidationSurvivesReceipt",
			rows: []Row{req, step(2, controlPart(t, a, ControlCandidate, `1`)), row(3, user, VisibilityModel, controlPart(t, a, ControlInvalidation, "")), row(4, assistant, VisibilityBoth, outcomePart(t, b))},
			want: ActiveRequestState{Active: true, RequestRowID: 1, GenerationSteps: 1, Invalidated: true},
		},
		{
			name: "ClosedStaysClosed",
			rows: append([]Row{req, step(2), row(3, assistant, VisibilityBoth, outcomePart(t, a))}, compaction...),
			want: ActiveRequestState{Active: true, RequestRowID: 1, GenerationSteps: 1, Closed: true, OutcomeRowID: 3},
		},
		{name: "ForeignControl", rows: []Row{req, step(2, controlPart(t, b, ControlRejection, ""))}, wantCorrupt: true},
		{name: "DuplicateOutcome", rows: []Row{req, row(2, assistant, VisibilityBoth, outcomePart(t, a)), row(3, system, VisibilityUser, outcomePart(t, a))}, wantCorrupt: true},
		{name: "ControlAfterOutcome", rows: []Row{req, row(2, assistant, VisibilityBoth, outcomePart(t, a), controlPart(t, a, ControlRejection, ""))}, wantCorrupt: true},
		{name: "TwoRequests", rows: []Row{row(1, user, VisibilityBoth, requestPart(t, a), requestPart(t, b))}, wantCorrupt: true},
		{name: "MalformedRequest", rows: []Row{row(1, user, VisibilityBoth, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeStructuredOutputRequest})}, wantCorrupt: true},
		{name: "MalformedControl", rows: []Row{req, row(2, assistant, VisibilityBoth, codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeStructuredOutputControl})}, wantCorrupt: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ActiveRequest(tt.rows)
			if tt.wantCorrupt {
				require.ErrorIs(t, err, ErrCorruptStructuredOutputState)
				require.Equal(t, ErrCorruptStructuredOutputState.Error(), err.Error())
				return
			}
			require.NoError(t, err)
			if tt.want.Active {
				tt.want.Request, err = DecodeRequestPart(req.Parts[1])
				require.NoError(t, err)
			}
			if tt.want.Closed {
				out, err := DecodeOutcomePart(outcomePart(t, a))
				require.NoError(t, err)
				tt.want.Outcome = &out
			}
			require.Equal(t, tt.want, got)
		})
	}
	// A corrupt state still identifies the request for a failure receipt.
	got, err := ActiveRequest([]Row{req, step(2, controlPart(t, b, ControlRejection, ""))})
	require.ErrorIs(t, err, ErrCorruptStructuredOutputState)
	require.Equal(t, a, got.Request.RequestID)
}
