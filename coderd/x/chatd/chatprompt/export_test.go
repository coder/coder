package chatprompt

import (
	"charm.land/fantasy"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

// SyntheticPasteTitleBudgetForTest exposes syntheticPasteTitleBudget
// for external tests.
const SyntheticPasteTitleBudgetForTest = syntheticPasteTitleBudget

// ToolResultPartToMessagePartForTest exposes toolResultPartToMessagePart
// for external tests.
var ToolResultPartToMessagePartForTest = toolResultPartToMessagePart

// EncodeNulInJSONForTest exposes encodeNulInJSON for external tests.
var EncodeNulInJSONForTest = encodeNulInJSON

// NeedsNulEncodingInJSONForTest exposes needsNulEncodingInJSON for external tests.
var NeedsNulEncodingInJSONForTest = needsNulEncodingInJSON

// ToolResultContentToPartForTest exposes toolResultContentToPart
// for external tests.
var ToolResultContentToPartForTest = func(logger slog.Logger, content fantasy.ToolResultContent) codersdk.ChatMessagePart {
	return toolResultContentToPart(logger, content, nil)
}
