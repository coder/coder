package chatprompt

import (
	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk"
)

// IsSyntheticPasteForTest exposes isSyntheticPaste for external tests.
var IsSyntheticPasteForTest = isSyntheticPaste

// ToolResultPartToMessagePartForTest exposes toolResultPartToMessagePart
// for external tests.
var ToolResultPartToMessagePartForTest = toolResultPartToMessagePart

// ToolResultContentToPartForTest exposes toolResultContentToPart
// for external tests.
var ToolResultContentToPartForTest = func(content fantasy.ToolResultContent) codersdk.ChatMessagePart {
	return toolResultContentToPart(content, nil)
}
