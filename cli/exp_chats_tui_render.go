package cli

import (
	"fmt"

	"github.com/coder/coder/v2/codersdk"
)

func renderToolCall(styles tuiStyles, part codersdk.ChatMessagePart, width int) string {
	_ = styles
	_ = width
	return fmt.Sprintf("[tool-call: %s]", part.ToolName)
}

func renderToolResult(styles tuiStyles, part codersdk.ChatMessagePart, width int) string {
	_ = styles
	_ = width
	return fmt.Sprintf("[tool-result: %s]", part.ToolName)
}

func renderCompaction(styles tuiStyles, width int) string {
	_ = styles
	_ = width
	return "--- context compacted ---"
}

func renderDiffDrawer(styles tuiStyles, diff codersdk.ChatDiffContents, width, height int) string {
	_ = styles
	_ = diff
	_ = width
	_ = height
	return "[diff drawer placeholder]"
}

func renderModelPicker(styles tuiStyles, catalog codersdk.ChatModelsResponse, selected string, width, height int) string {
	_ = styles
	_ = catalog
	_ = selected
	_ = width
	_ = height
	return "[model picker placeholder]"
}

func messagesToBlocks(messages []codersdk.ChatMessage) []chatBlock {
	_ = messages
	return nil
}
