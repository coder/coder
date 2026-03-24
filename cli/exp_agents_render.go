package cli

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/coder/coder/v2/codersdk"
)

const contextCompactionToolName = "context_compaction"

func renderToolCall(styles tuiStyles, part codersdk.ChatMessagePart, width int) string {
	if part.ToolName == contextCompactionToolName {
		return renderCompaction(styles, width)
	}

	toolName := part.ToolName
	if toolName == "" {
		toolName = "tool"
	}

	label := styles.toolCallStyle.Render("🔧 " + toolName)
	args := compactTranscriptJSON(part.Args)
	if args == "" || width <= 0 {
		return label
	}

	available := width - lipgloss.Width(label) - 1
	preview := styles.truncate(args, max(available, 0))
	if preview == "" {
		return label
	}
	return label + " " + styles.dimmedText.Render(preview)
}

func renderToolResult(styles tuiStyles, part codersdk.ChatMessagePart, width int) string {
	if part.ToolName == contextCompactionToolName {
		return renderCompaction(styles, width)
	}

	toolName := part.ToolName
	if toolName == "" {
		toolName = "tool"
	}

	icon := "✓"
	labelStyle := styles.toolCallStyle
	if part.IsError {
		icon = "✗"
		labelStyle = styles.errorText
	}

	label := labelStyle.Render(icon + " " + toolName)
	result := compactTranscriptJSON(part.Result)
	if result == "" {
		result = "null"
	}
	if width <= 0 {
		return label + " " + styles.dimmedText.Render(result)
	}

	available := width - lipgloss.Width(label) - 1
	preview := styles.truncate(result, max(available, 0))
	if preview == "" {
		return label
	}
	return label + " " + styles.dimmedText.Render(preview)
}

func renderCompaction(styles tuiStyles, width int) string {
	banner := styles.compaction.Render("🗜️  Context compacted")
	if width <= 0 {
		return banner
	}
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, banner)
}

func renderDiffDrawer(styles tuiStyles, diff codersdk.ChatDiffContents, changes []codersdk.ChatGitChange, width, height int) string {
	innerWidth := 80
	if width > 0 {
		innerWidth = max(width-6, 1)
	}

	headerBits := []string{styles.title.Render("Diff")}
	var meta []string
	if diff.Branch != nil && *diff.Branch != "" {
		meta = append(meta, fmt.Sprintf("Branch: %s", *diff.Branch))
	}
	if diff.PullRequestURL != nil && *diff.PullRequestURL != "" {
		meta = append(meta, fmt.Sprintf("PR: %s", *diff.PullRequestURL))
	}
	if len(meta) > 0 {
		headerBits = append(headerBits, styles.subtitle.Render(strings.Join(meta, " • ")))
	}

	summary := renderChatDiffSummary(diff, changes)
	diffBody := diff.Diff
	if strings.TrimSpace(diffBody) == "" {
		diffBody = styles.dimmedText.Render("No diff contents.")
	}

	help := styles.helpText.Render("Esc to close")
	overhead := countRenderedLines(strings.Join(headerBits, "\n")) + countRenderedLines(summary) + countRenderedLines(help) + 4
	availableBodyLines := height - overhead
	if height <= 0 {
		availableBodyLines = 12
	}
	if availableBodyLines < 0 {
		availableBodyLines = 0
	}

	wrappedDiff := wrapPreservingNewlines(diffBody, innerWidth)
	if availableBodyLines == 0 {
		wrappedDiff = ""
	} else {
		wrappedDiff = clampLines(wrappedDiff, availableBodyLines)
	}

	sections := []string{strings.Join(headerBits, "\n")}
	if summary != "" {
		sections = append(sections, summary)
	}
	if wrappedDiff != "" {
		sections = append(sections, wrappedDiff)
	}
	sections = append(sections, help)

	panel := strings.Join(sections, "\n\n")
	frame := styles.overlayBorder.Width(innerWidth)
	return frame.Render(panel)
}

func renderModelPicker(styles tuiStyles, catalog codersdk.ChatModelsResponse, selected string, cursor int, width, height int) string {
	innerWidth := 80
	if width > 0 {
		innerWidth = max(width-6, 1)
	}

	lines := []string{styles.title.Render("Select Model")}
	flatIndex := 0
	for _, provider := range catalog.Providers {
		lines = append(lines, styles.subtitle.Render(provider.Provider))
		if !provider.Available {
			reason := string(provider.UnavailableReason)
			if reason == "" {
				reason = "unavailable"
			}
			lines = append(lines, "  "+styles.dimmedText.Render(reason))
			lines = append(lines, "")
			continue
		}

		if len(provider.Models) == 0 {
			lines = append(lines, "  "+styles.dimmedText.Render("No models available."))
			lines = append(lines, "")
			continue
		}

		for _, model := range provider.Models {
			name := model.DisplayName
			if strings.TrimSpace(name) == "" {
				name = model.Model
			}
			marker := "  "
			if flatIndex == cursor {
				marker = "> "
			}

			rowStyle := styles.normalItem
			if model.ID == selected {
				rowStyle = styles.selectedItem
			}
			lines = append(lines, marker+rowStyle.Render(styles.truncate(name, max(innerWidth-2, 0))))
			flatIndex++
		}
		lines = append(lines, "")
	}

	help := styles.helpText.Render("Esc to close, Enter to select")
	contentLines := lines
	maxContentLines := height - countRenderedLines(help) - 4
	if height <= 0 {
		maxContentLines = len(contentLines)
	}
	if maxContentLines < 1 {
		maxContentLines = 1
	}
	content := clampLineSlice(contentLines, maxContentLines)
	content = append(content, help)

	frame := styles.overlayBorder.Width(innerWidth)
	return frame.Render(strings.Join(content, "\n"))
}

//nolint:revive // Signature is dictated by the chat TUI view code.
func renderChatBlocks(styles tuiStyles, blocks []chatBlock, selectedBlock int, expandedBlocks map[int]bool, composerFocused bool, width int, renderers ...*glamour.TermRenderer) string {
	if len(blocks) == 0 {
		return ""
	}

	var renderer *glamour.TermRenderer
	if len(renderers) > 0 {
		renderer = renderers[0]
	}

	rendered := make([]string, 0, len(blocks))
	for i := range blocks {
		blockView := blocks[i].cachedRender
		if blockView == "" || blocks[i].cachedWidth != width || blocks[i].cachedExpanded != expandedBlocks[i] {
			blockView = renderBlock(styles, blocks[i], expandedBlocks[i], width, renderer)
			blocks[i].cachedRender = blockView
			blocks[i].cachedWidth = width
			blocks[i].cachedExpanded = expandedBlocks[i]
		}
		if !composerFocused && i == selectedBlock {
			blockView = styles.selectedItem.Render(blockView)
		}
		rendered = append(rendered, blockView)
	}

	return strings.Join(rendered, "\n")
}

//nolint:revive // Signature is dictated by the chat TUI view code.
func renderStatusBar(styles tuiStyles, chat *codersdk.Chat, status codersdk.ChatStatus, usage *codersdk.ChatMessageUsage, queueCount int, interrupting, reconnecting bool, width int) string {
	_ = chat

	parts := []string{styles.statusColor(status).Render(string(status))}

	if usage != nil && usage.TotalTokens != nil && usage.ContextLimit != nil {
		total := *usage.TotalTokens
		limit := *usage.ContextLimit
		if limit > 0 {
			tokenText := fmt.Sprintf("tokens: %d/%d", total, limit)
			pct := float64(total) / float64(limit) * 100
			switch {
			case pct > 95:
				tokenText = styles.criticalText.Render(tokenText)
			case pct > 80:
				tokenText = styles.warningText.Render(tokenText)
			}
			parts = append(parts, tokenText)
		}
	}

	if queueCount > 0 {
		parts = append(parts, fmt.Sprintf("queued: %d", queueCount))
	}
	if interrupting {
		parts = append(parts, styles.warningText.Render("interrupting…"))
	}
	if reconnecting {
		parts = append(parts, styles.warningText.Render("reconnecting…"))
	}

	line := strings.Join(parts, styles.separator.Render(" │ "))
	bar := styles.statusBar
	if width > 0 {
		bar = bar.MaxWidth(width)
	}
	return bar.Render(line)
}

func messagesToBlocks(messages []codersdk.ChatMessage) []chatBlock {
	blocks := make([]chatBlock, 0)
	for _, message := range messages {
		if message.Role == codersdk.ChatMessageRoleSystem {
			continue
		}

		for _, part := range message.Content {
			switch part.Type {
			case codersdk.ChatMessagePartTypeText:
				blocks = append(blocks, chatBlock{kind: blockText, role: message.Role, text: part.Text})
			case codersdk.ChatMessagePartTypeReasoning:
				blocks = append(blocks, chatBlock{kind: blockReasoning, role: message.Role, text: part.Text})
			case codersdk.ChatMessagePartTypeToolCall:
				kind := blockToolCall
				if part.ToolName == contextCompactionToolName {
					kind = blockCompaction
				}
				blocks = append(blocks, chatBlock{
					kind:     kind,
					role:     message.Role,
					toolName: part.ToolName,
					toolID:   part.ToolCallID,
					args:     compactTranscriptJSON(part.Args),
				})
			case codersdk.ChatMessagePartTypeToolResult:
				kind := blockToolResult
				if part.ToolName == contextCompactionToolName {
					kind = blockCompaction
				}
				blocks = append(blocks, chatBlock{
					kind:     kind,
					role:     message.Role,
					toolName: part.ToolName,
					toolID:   part.ToolCallID,
					result:   compactTranscriptJSON(part.Result),
					isError:  part.IsError,
				})
			case codersdk.ChatMessagePartTypeSource:
				title := part.Title
				if strings.TrimSpace(title) == "" {
					title = part.URL
				}
				blocks = append(blocks, chatBlock{kind: blockText, role: message.Role, text: fmt.Sprintf("[Source: %s](%s)", title, part.URL)})
			case codersdk.ChatMessagePartTypeFile:
				blocks = append(blocks, chatBlock{kind: blockText, role: message.Role, text: fmt.Sprintf("[File: %s]", part.MediaType)})
			case codersdk.ChatMessagePartTypeFileReference:
				blocks = append(blocks, chatBlock{kind: blockText, role: message.Role, text: fmt.Sprintf("[%s L%d-%d]", part.FileName, part.StartLine, part.EndLine)})
			}
		}
	}
	return blocks
}

//nolint:revive // Signature keeps block expansion state explicit at the callsite.
func renderBlock(styles tuiStyles, block chatBlock, expanded bool, width int, renderers ...*glamour.TermRenderer) string {
	var renderer *glamour.TermRenderer
	if len(renderers) > 0 {
		renderer = renderers[0]
	}

	switch block.kind {
	case blockText:
		switch block.role {
		case codersdk.ChatMessageRoleUser:
			return renderPrefixedBlock(styles.userMessage.Render("You: "), block.text, width)
		case codersdk.ChatMessageRoleAssistant:
			return renderAssistantMarkdown(styles, block.text, width, renderer)
		case codersdk.ChatMessageRoleTool:
			return styles.dimmedText.Render(wrapPreservingNewlines(block.text, width))
		default:
			return wrapPreservingNewlines(block.text, width)
		}
	case blockReasoning:
		content := wrapPreservingNewlines("💭 "+block.text, width)
		if !expanded {
			content = clampLines(content, 3)
		}
		return styles.reasoning.Render(content)
	case blockToolCall:
		if !expanded {
			return renderToolCall(styles, codersdk.ChatMessagePart{ToolName: block.toolName, Args: []byte(block.args)}, width)
		}
		lines := []string{styles.toolCallStyle.Render("🔧 " + block.toolName)}
		if strings.TrimSpace(block.args) != "" {
			lines = append(lines, wrapPreservingNewlines(block.args, width))
		}
		return strings.Join(lines, "\n")
	case blockToolResult:
		if !expanded {
			return renderToolResult(styles, codersdk.ChatMessagePart{ToolName: block.toolName, Result: []byte(block.result), IsError: block.isError}, width)
		}
		icon := "✓"
		labelStyle := styles.toolCallStyle
		if block.isError {
			icon = "✗"
			labelStyle = styles.errorText
		}
		lines := []string{labelStyle.Render(icon + " " + block.toolName)}
		result := block.result
		if strings.TrimSpace(result) == "" {
			result = "null"
		}
		lines = append(lines, wrapPreservingNewlines(result, width))
		return strings.Join(lines, "\n")
	case blockCompaction:
		return renderCompaction(styles, width)
	default:
		return ""
	}
}

var fallbackMarkdownRenderers sync.Map

func getFallbackMarkdownRenderer(width int) *glamour.TermRenderer {
	wrapWidth := width
	if wrapWidth <= 0 {
		wrapWidth = 80
	}
	if cachedRenderer, ok := fallbackMarkdownRenderers.Load(wrapWidth); ok {
		renderer, ok := cachedRenderer.(*glamour.TermRenderer)
		if ok {
			return renderer
		}
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(wrapWidth),
	)
	if err != nil {
		return nil
	}
	cachedRenderer, _ := fallbackMarkdownRenderers.LoadOrStore(wrapWidth, renderer)
	storedRenderer, ok := cachedRenderer.(*glamour.TermRenderer)
	if !ok {
		return nil
	}
	return storedRenderer
}

func renderAssistantMarkdown(styles tuiStyles, text string, width int, renderers ...*glamour.TermRenderer) string {
	var renderer *glamour.TermRenderer
	if len(renderers) > 0 {
		renderer = renderers[0]
	}
	if renderer == nil {
		renderer = getFallbackMarkdownRenderer(width)
	}
	if renderer != nil {
		rendered, err := renderer.Render(text)
		if err == nil {
			return styles.assistantMsg.Render(strings.TrimRight(rendered, "\n"))
		}
	}
	return styles.assistantMsg.Render(wrapPreservingNewlines(text, width))
}

func renderPrefixedBlock(prefix, body string, width int) string {
	if strings.TrimSpace(body) == "" {
		return prefix
	}
	prefixWidth := lipgloss.Width(prefix)
	available := width - prefixWidth
	if available <= 0 {
		available = width
	}
	wrapped := wrapPreservingNewlines(body, available)
	lines := strings.Split(wrapped, "\n")
	if len(lines) == 0 {
		return prefix
	}
	for i := 1; i < len(lines); i++ {
		lines[i] = strings.Repeat(" ", max(prefixWidth, 0)) + lines[i]
	}
	return prefix + strings.Join(lines, "\n")
}

func wrapPreservingNewlines(text string, width int) string {
	if width <= 0 {
		return text
	}
	style := lipgloss.NewStyle().Width(width)
	segments := strings.Split(text, "\n")
	for i, segment := range segments {
		segments[i] = strings.TrimRight(style.Render(segment), " ")
	}
	return strings.Join(segments, "\n")
}

func clampLines(text string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text
	}
	lines = lines[:maxLines]
	lines[maxLines-1] = stylesafeEllipsis(lines[maxLines-1])
	return strings.Join(lines, "\n")
}

func clampLineSlice(lines []string, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	if len(lines) <= maxLines {
		return lines
	}
	clamped := append([]string(nil), lines[:maxLines]...)
	clamped[maxLines-1] = stylesafeEllipsis(clamped[maxLines-1])
	return clamped
}

func stylesafeEllipsis(line string) string {
	trimmed := strings.TrimRight(line, " ")
	if trimmed == "" {
		return "…"
	}
	return trimmed + "…"
}

func countRenderedLines(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}
