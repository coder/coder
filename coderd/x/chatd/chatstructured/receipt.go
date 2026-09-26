package chatstructured

import (
	"github.com/coder/coder/v2/codersdk"
)

// ReceiptParts builds the content of a terminal receipt row: a text
// fallback for readers that do not understand outcome parts, then exactly
// one outcome part. The fallback of a success is the output's JSON text;
// otherwise it is a fixed sentence naming the status and error code, never
// the error message, which may quote provider responses.
func ReceiptParts(out codersdk.ChatStructuredOutput) ([]codersdk.ChatMessagePart, error) {
	outcome, err := EncodeOutcomePart(out)
	if err != nil {
		return nil, err
	}
	fallback := string(out.Value)
	if out.Status != codersdk.ChatStructuredOutputStatusSucceeded {
		fallback = "The structured output request " + string(out.Status) + " (" + string(out.Error.Code) + ")."
	}
	return []codersdk.ChatMessagePart{codersdk.ChatMessageText(fallback), outcome}, nil
}

// IsReceipt reports whether parts have the receipt shape: exactly one
// outcome part and otherwise only text parts. It mirrors the database
// predicate that exempts receipt rows from execution fencing.
func IsReceipt(parts []codersdk.ChatMessagePart) bool {
	outcomes := 0
	for _, part := range parts {
		switch part.Type {
		case codersdk.ChatMessagePartTypeStructuredOutputOutcome:
			outcomes++
		case codersdk.ChatMessagePartTypeText:
		default:
			return false
		}
	}
	return outcomes == 1
}

// ReceiptOutcome decodes the outcome of receipt-shaped parts, returning
// ErrMalformedStructuredOutputMetadata for any other parts.
func ReceiptOutcome(parts []codersdk.ChatMessagePart) (codersdk.ChatStructuredOutput, error) {
	if !IsReceipt(parts) {
		return codersdk.ChatStructuredOutput{}, ErrMalformedStructuredOutputMetadata
	}
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeStructuredOutputOutcome {
			return DecodeOutcomePart(part)
		}
	}
	return codersdk.ChatStructuredOutput{}, ErrMalformedStructuredOutputMetadata
}
