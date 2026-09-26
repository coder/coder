// This file reconstructs a chat's active structured output request from its
// history. It is pure: callers pass chronological rows and get the state
// that decides what the executor does next, or a distinct error when the
// stored metadata is inconsistent.

package chatstructured

import (
	"encoding/json"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// ErrCorruptStructuredOutputState reports inconsistent structured output
// metadata in history, as opposed to having no active request.
var ErrCorruptStructuredOutputState = xerrors.New("structured output history is inconsistent")

// Visibility mirrors chat message visibility: who sees a row.
type Visibility string

const (
	VisibilityUser  Visibility = "user"
	VisibilityModel Visibility = "model"
	VisibilityBoth  Visibility = "both"
)

// Row is one chat message as state reconstruction needs it. ID is an opaque
// row identifier that callers use for fences and receipts.
type Row struct {
	ID         int64
	Role       codersdk.ChatMessageRole
	Visibility Visibility
	Parts      []codersdk.ChatMessagePart
}

// ActiveRequestState describes the structured output request of the latest
// user turn. Active is false when that turn asked for ordinary text.
type ActiveRequestState struct {
	Active       bool
	Request      Request
	RequestRowID int64
	// Rejections counts rejected finalizer calls.
	Rejections int
	// Candidate is the latest valid candidate output not since invalidated.
	Candidate json.RawMessage
	// GenerationSteps counts client-visible assistant rows after the request
	// with content besides control parts and no outcome part. Compaction's
	// display row is user-only and is not counted.
	GenerationSteps int
	Closed          bool
	Outcome         *codersdk.ChatStructuredOutput
	OutcomeRowID    int64
}

// ActiveRequest reconstructs the request state from chronological history,
// compressed rows included. The request belongs to the latest real user row;
// model-only copies made by compaction never start or revive one. Outcomes
// for other requests close those requests and are skipped. Any other
// inconsistency returns ErrCorruptStructuredOutputState; once the request row
// held exactly one valid request part, the returned state still carries it.
func ActiveRequest(rows []Row) (ActiveRequestState, error) {
	i := len(rows) - 1
	for i >= 0 && !realUserRow(rows[i]) {
		i--
	}
	var state ActiveRequestState
	if i < 0 {
		return state, nil
	}
	for _, part := range rows[i].Parts {
		if part.Type != codersdk.ChatMessagePartTypeStructuredOutputRequest {
			continue
		}
		req, err := DecodeRequestPart(part)
		if err != nil || state.Active {
			return ActiveRequestState{}, ErrCorruptStructuredOutputState
		}
		state = ActiveRequestState{Active: true, Request: req, RequestRowID: rows[i].ID}
	}
	if !state.Active {
		return state, nil
	}
	for _, row := range rows[i+1:] {
		content, outcome := false, false
		for _, part := range row.Parts {
			switch part.Type {
			case codersdk.ChatMessagePartTypeStructuredOutputOutcome:
				out, err := DecodeOutcomePart(part)
				if err != nil {
					return state, ErrCorruptStructuredOutputState
				}
				outcome = true
				if out.RequestID != state.Request.RequestID {
					continue
				}
				if state.Closed {
					return state, ErrCorruptStructuredOutputState
				}
				state.Closed, state.Outcome, state.OutcomeRowID = true, &out, row.ID
			case codersdk.ChatMessagePartTypeStructuredOutputControl:
				c, err := DecodeControlPart(part)
				if err != nil || c.RequestID != state.Request.RequestID || state.Closed {
					return state, ErrCorruptStructuredOutputState
				}
				switch c.Kind {
				case ControlCandidate:
					state.Candidate = c.Value
				case ControlRejection:
					state.Rejections++
				case ControlInvalidation:
					state.Candidate = nil
				}
			default:
				content = true
			}
		}
		if row.Role == codersdk.ChatMessageRoleAssistant && row.Visibility == VisibilityBoth && content && !outcome {
			state.GenerationSteps++
		}
	}
	return state, nil
}

// realUserRow reports whether row is a client-visible user turn. Standalone
// hook notices are system rows visible to the user and turn-time hook context
// is a model-only user row (chathooks EventMessages), so neither qualifies;
// user rows holding only notices or structured output metadata are skipped.
func realUserRow(row Row) bool {
	if row.Role != codersdk.ChatMessageRoleUser || (row.Visibility != VisibilityUser && row.Visibility != VisibilityBoth) {
		return false
	}
	content := false
	for _, part := range row.Parts {
		switch part.Type {
		case codersdk.ChatMessagePartTypeStructuredOutputControl, codersdk.ChatMessagePartTypeStructuredOutputOutcome:
			return false
		case codersdk.ChatMessagePartTypeHookNotice:
		default:
			content = true
		}
	}
	return content
}
