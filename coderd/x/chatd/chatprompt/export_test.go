package chatprompt

import (
	"charm.land/fantasy"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

// IsSyntheticPasteForTest exposes isSyntheticPaste for external tests.
var IsSyntheticPasteForTest = isSyntheticPaste

// ToolResultPartToMessagePartForTest exposes toolResultPartToMessagePart
// for external tests.
var ToolResultPartToMessagePartForTest = toolResultPartToMessagePart

// ToolResultContentToPartForTest exposes toolResultContentToPart
// for external tests.
var ToolResultContentToPartForTest = func(logger slog.Logger, content fantasy.ToolResultContent) codersdk.ChatMessagePart {
	return toolResultContentToPart(logger, content, nil)
}
