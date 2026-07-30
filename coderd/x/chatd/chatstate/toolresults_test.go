package chatstate_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
)

func TestValidateToolResults(t *testing.T) {
	t.Parallel()

	// Validation order is part of the contract because callers surface only
	// the first violation.
	pending := map[string]string{"call_a": "execute", "call_b": "read_file"}
	resultA := chatstate.ToolResultInput{ToolCallID: "call_a", Output: json.RawMessage(`{"ok":true}`)}
	resultB := chatstate.ToolResultInput{ToolCallID: "call_b", Output: json.RawMessage(`"done"`)}
	badJSON := chatstate.ToolResultInput{ToolCallID: "call_b", Output: json.RawMessage(`{`)}
	resultC := chatstate.ToolResultInput{ToolCallID: "call_c", Output: json.RawMessage(`{}`)}

	cases := []struct {
		name           string
		results        []chatstate.ToolResultInput
		wantCause      error
		wantToolCallID string
	}{
		{
			name:    "Complete",
			results: []chatstate.ToolResultInput{resultA, resultB},
		},
		{
			name:           "Duplicate",
			results:        []chatstate.ToolResultInput{resultA, resultB, resultA},
			wantCause:      chatstate.ErrToolResultDuplicate,
			wantToolCallID: "call_a",
		},
		{
			name:           "InvalidJSON",
			results:        []chatstate.ToolResultInput{resultA, badJSON},
			wantCause:      chatstate.ErrToolResultInvalidJSON,
			wantToolCallID: "call_b",
		},
		{
			name:           "Missing",
			results:        []chatstate.ToolResultInput{resultA},
			wantCause:      chatstate.ErrToolResultMissing,
			wantToolCallID: "call_b",
		},
		{
			name:           "Unexpected",
			results:        []chatstate.ToolResultInput{resultA, resultB, resultC},
			wantCause:      chatstate.ErrToolResultUnexpected,
			wantToolCallID: "call_c",
		},
		{
			name:           "PerResultRulesOutrankSweeps",
			results:        []chatstate.ToolResultInput{resultC, badJSON},
			wantCause:      chatstate.ErrToolResultInvalidJSON,
			wantToolCallID: "call_b",
		},
		{
			name:           "MissingOutranksUnexpected",
			results:        []chatstate.ToolResultInput{resultA, resultC},
			wantCause:      chatstate.ErrToolResultMissing,
			wantToolCallID: "call_b",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			invalid := chatstate.ValidateToolResults(test.results, pending)
			if test.wantCause == nil {
				require.Nil(t, invalid)
				return
			}
			require.NotNil(t, invalid)
			require.ErrorIs(t, invalid, test.wantCause)
			require.Equal(t, test.wantToolCallID, invalid.ToolCallID)
		})
	}
}
