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

func renderDiffDrawer(styles tuiStyles, diff codersdk.ChatDiffContents, changes []codersdk.ChatGitChange, width, height int) string {
	_ = styles
	_ = diff
	_ = changes
	_ = width
	_ = height
	return "[diff drawer placeholder]"
}

func renderModelPicker(styles tuiStyles, catalog codersdk.ChatModelsResponse, selected string, cursor int, width, height int) string {
	_ = styles
	_ = catalog
	_ = selected
	_ = cursor
	_ = width
	_ = height
	return "[model picker placeholder]"
}

func renderChatBlocks(styles tuiStyles, blocks []chatBlock, selectedBlock int, expandedBlocks map[int]bool, composerFocused bool, width int) string {
	_ = styles
	_ = blocks
	_ = selectedBlock
	_ = expandedBlocks
	_ = composerFocused
	_ = width
	return "[Chat blocks will be rendered here]"
}

func renderStatusBar(styles tuiStyles, chat *codersdk.Chat, status codersdk.ChatStatus, usage *codersdk.ChatMessageUsage, queueCount int, interrupting, reconnecting bool, width int) string {
	_ = styles
	_ = chat
	_ = status
	_ = usage
	_ = queueCount
	_ = interrupting
	_ = reconnecting
	_ = width
	return ""
}

func messagesToBlocks(messages []codersdk.ChatMessage) []chatBlock {
	_ = messages
	return nil
}
