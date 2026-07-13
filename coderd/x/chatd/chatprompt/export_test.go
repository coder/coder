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

// ToolResultContentToPartForTest exposes toolResultContentToPart
// for external tests.
var ToolResultContentToPartForTest = func(logger slog.Logger, content fantasy.ToolResultContent) codersdk.ChatMessagePart {
	return toolResultContentToPart(logger, content, nil)
}
