package chatstructured

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestReceiptParts(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	succeeded := codersdk.ChatStructuredOutput{RequestID: id, Status: codersdk.ChatStructuredOutputStatusSucceeded, Value: json.RawMessage(`{"n":9007199254740993}`)}
	failed := codersdk.ChatStructuredOutput{RequestID: id, Status: codersdk.ChatStructuredOutputStatusFailed, Error: &codersdk.ChatStructuredOutputError{
		Code: codersdk.ChatStructuredOutputErrorCodeValidationExhausted, Message: "provider said SECRET",
	}}
	for out, fallback := range map[*codersdk.ChatStructuredOutput]string{
		&succeeded: `{"n":9007199254740993}`,
		&failed:    "The structured output request failed (validation_exhausted).",
	} {
		parts, err := ReceiptParts(*out)
		require.NoError(t, err)
		require.Len(t, parts, 2)
		require.Equal(t, codersdk.ChatMessageText(fallback), parts[0])
		require.True(t, IsReceipt(parts))
		got, err := ReceiptOutcome(parts)
		require.NoError(t, err)
		require.Equal(t, *out, got)
	}
	_, err := ReceiptParts(codersdk.ChatStructuredOutput{RequestID: id, Status: codersdk.ChatStructuredOutputStatusSucceeded})
	require.ErrorIs(t, err, ErrMalformedStructuredOutputMetadata)

	outcome, err := EncodeOutcomePart(succeeded)
	require.NoError(t, err)
	text := codersdk.ChatMessageText("x")
	control := codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeStructuredOutputControl}
	malformed := codersdk.ChatMessagePart{Type: codersdk.ChatMessagePartTypeStructuredOutputOutcome, StructuredOutputData: json.RawMessage(`{}`)}
	// IsReceipt checks the shape only; ReceiptOutcome also decodes it.
	for _, tt := range []struct {
		parts        []codersdk.ChatMessagePart
		shape, valid bool
	}{
		{[]codersdk.ChatMessagePart{outcome}, true, true},
		{[]codersdk.ChatMessagePart{text, outcome, text}, true, true},
		{[]codersdk.ChatMessagePart{malformed}, true, false},
		{nil, false, false},
		{[]codersdk.ChatMessagePart{text}, false, false},
		{[]codersdk.ChatMessagePart{outcome, outcome}, false, false},
		{[]codersdk.ChatMessagePart{outcome, control}, false, false},
		{[]codersdk.ChatMessagePart{outcome, codersdk.ChatMessageToolCall("c", "t", nil)}, false, false},
	} {
		require.Equal(t, tt.shape, IsReceipt(tt.parts))
		_, err := ReceiptOutcome(tt.parts)
		require.Equal(t, tt.valid, err == nil)
	}
}
