package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/coder/coder/v2/codersdk"
)

const (
	contextCompactionToolName = "context_compaction"
	toolBlockIndent           = "  "
	toolDetailIndent          = "    "
	toolSummaryFallbackWidth  = 48
)

func humanizeToolName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "coder_")
	name = strings.TrimPrefix(name, "github__")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "tool"
	}
	return name
}

func normalizeToolName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "coder_")
	name = strings.TrimPrefix(name, "github__")
	return strings.ToLower(name)
}

func toolArgsSummary(toolName string, argsJSON string) string {
	raw := strings.TrimSpace(argsJSON)
	if raw == "" {
		return ""
	}

	parsed, ok := parseToolSummaryValue(raw)
	if ok {
		if summary := toolObjectSummary(toolName, parsed); summary != "" {
			return summary
		}
		if value := firstShortStringValue(parsed); value != "" {
			return strconv.Quote(value)
		}
	}

	compact := compactTranscriptJSON(json.RawMessage(raw))
	if compact == "" {
		return ""
	}
	return truncateToolSummary(compact, toolSummaryFallbackWidth)
}

func toolObjectSummary(toolName string, parsed any) string {
	normalized := normalizeToolName(toolName)
	switch {
	case normalized == "execute" || normalized == "execute_command" || normalized == "run_command":
		if command := firstStringField(parsed, "command", "cmd", "script", "input"); command != "" {
			return strconv.Quote(command)
		}
	case strings.Contains(normalized, "read_file") || strings.Contains(normalized, "write_file") || strings.Contains(normalized, "delete_file") || strings.Contains(normalized, "stat_file"):
		if path := firstStringField(parsed, "path", "file_path", "filename"); path != "" {
			return "(" + path + ")"
		}
	case normalized == "get_pull_request":
		owner := firstStringField(parsed, "owner")
		repo := firstStringField(parsed, "repo", "repository")
		switch {
		case owner != "" && repo != "":
			return "(" + owner + "/" + repo + ")"
		case repo != "":
			return "(" + repo + ")"
		}
	case strings.Contains(normalized, "workspace"):
		if workspace := firstStringField(parsed, "workspace_name", "name", "workspace"); workspace != "" {
			return "(" + workspace + ")"
		}
	}

	return ""
}

func toolResultSummary(toolName, argsJSON, resultJSON string) string {
	if summary := toolArgsSummary(toolName, argsJSON); summary != "" {
		return summary
	}

	compact := compactTranscriptJSON(json.RawMessage(resultJSON))
	if compact == "" {
		return "null"
	}

	if parsed, ok := parseToolSummaryValue(compact); ok {
		if summary := toolObjectSummary(toolName, parsed); summary != "" {
			return summary
		}
		if value := firstShortStringValue(parsed); value != "" {
			return strconv.Quote(value)
		}
	}

	return truncateToolSummary(compact, toolSummaryFallbackWidth)
}

func toolErrorSummary(resultJSON string) string {
	raw := strings.TrimSpace(resultJSON)
	if raw == "" {
		return ""
	}

	if parsed, ok := parseToolSummaryValue(raw); ok {
		if message := firstStringField(parsed, "error", "message", "detail", "stderr"); message != "" {
			return strconv.Quote(message)
		}
		if value := firstShortStringValue(parsed); value != "" {
			return strconv.Quote(value)
		}
	}

	compact := compactTranscriptJSON(json.RawMessage(raw))
	if compact == "" {
		return ""
	}
	return truncateToolSummary(compact, toolSummaryFallbackWidth)
}

func parseToolSummaryValue(raw string) (any, bool) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, false
	}
	return value, true
}

func firstStringField(value any, keys ...string) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}

	for _, key := range keys {
		fieldValue, ok := object[key]
		if !ok {
			continue
		}
		if text := firstShortStringValue(fieldValue); text != "" {
			return text
		}
	}

	return ""
}

func firstShortStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		trimmed := strings.Join(strings.Fields(strings.TrimSpace(typed)), " ")
		if trimmed == "" {
			return ""
		}
		return trimmed
	case []any:
		for _, item := range typed {
			if text := firstShortStringValue(item); text != "" {
				return text
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			if text := firstShortStringValue(typed[key]); text != "" {
				return text
			}
		}
	}

	return ""
}

func truncateToolSummary(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if len([]rune(text)) <= maxWidth {
		return text
	}
	return string([]rune(text)[:maxWidth-1]) + "…"
}

func renderToolLine(styles tuiStyles, labelStyle lipgloss.Style, icon, toolName, summary string, width int) string {
	header := toolBlockIndent + labelStyle.Render(icon) + " " + humanizeToolName(toolName)
	if summary == "" || width <= 0 {
		return header
	}

	available := width - lipgloss.Width(header) - 1
	preview := styles.truncate(summary, max(available, 0))
	if preview == "" {
		return header
	}

	return header + " " + styles.dimmedText.Render(preview)
}

func renderToolDetail(styles tuiStyles, label, value string, width int) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}

	prefix := toolDetailIndent + label + ": "
	contentWidth := 80
	if width > 0 {
		contentWidth = max(width-lipgloss.Width(prefix), 1)
	}
	wrapped := wrapPreservingNewlines(value, contentWidth)
	lines := strings.Split(wrapped, "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = prefix + lines[i]
			continue
		}
		lines[i] = strings.Repeat(" ", lipgloss.Width(prefix)) + lines[i]
	}

	return styles.dimmedText.Render(strings.Join(lines, "\n"))
}

func renderExpandedToolBlock(styles tuiStyles, labelStyle lipgloss.Style, icon, toolName, args, result string, width int) string {
	lines := []string{toolBlockIndent + labelStyle.Render(icon) + " " + humanizeToolName(toolName)}
	if argsLine := renderToolDetail(styles, "args", args, width); argsLine != "" {
		lines = append(lines, argsLine)
	}
	if result != "" {
		if resultLine := renderToolDetail(styles, "result", result, width); resultLine != "" {
			lines = append(lines, resultLine)
		}
	}
	return strings.Join(lines, "\n")
}

func renderToolCall(styles tuiStyles, part codersdk.ChatMessagePart, width int) string {
	if part.ToolName == contextCompactionToolName {
		return renderCompaction(styles, width)
	}

	return renderToolLine(styles, styles.toolPending, "⏳", part.ToolName, toolArgsSummary(part.ToolName, compactTranscriptJSON(part.Args)), width)
}

func renderToolResult(styles tuiStyles, part codersdk.ChatMessagePart, width int) string {
	if part.ToolName == contextCompactionToolName {
		return renderCompaction(styles, width)
	}

	icon := "✓"
	labelStyle := styles.toolSuccess
	if part.IsError {
		icon = "✗"
		labelStyle = styles.errorText
	}

	argsJSON := compactTranscriptJSON(part.Args)
	resultJSON := compactTranscriptJSON(part.Result)
	summary := toolArgsSummary(part.ToolName, argsJSON)
	if summary == "" {
		summary = toolResultSummary(part.ToolName, "", resultJSON)
		if part.IsError {
			if errorSummary := toolErrorSummary(resultJSON); errorSummary != "" {
				summary = errorSummary
			}
		}
	}
	return renderToolLine(styles, labelStyle, icon, part.ToolName, summary, width)
}

func renderCompaction(styles tuiStyles, width int) string {
	banner := styles.compaction.Render("🗜️  Context compacted")
	if width <= 0 {
		return banner
	}
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, banner)
}

func renderDiffDrawerLoading(styles tuiStyles, width, _ int) string {
	return renderDiffDrawerState(styles, styles.dimmedText.Render("Loading diff…"), width)
}

func renderDiffDrawerError(styles tuiStyles, err error, width, _ int) string {
	message := styles.errorText.Render("Failed to load diff.")
	if err != nil {
		message = styles.errorText.Render(wrapPreservingNewlines(err.Error(), max(width-6, 1)))
	}
	return renderDiffDrawerState(styles, message, width)
}

func renderDiffDrawerState(styles tuiStyles, body string, width int) string {
	innerWidth := 80
	if width > 0 {
		innerWidth = max(width-6, 1)
	}

	sections := []string{
		styles.title.Render("Diff"),
		body,
		styles.helpText.Render("Esc to close"),
	}
	frame := styles.overlayBorder.Width(innerWidth)
	return frame.Render(strings.Join(sections, "\n\n"))
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
	hasModels := false
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
			hasModels = true
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

	if !hasModels {
		lines = append(lines, styles.dimmedText.Render("No models available."))
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
	return mergeConsecutiveToolBlocks(blocks)
}

func mergeConsecutiveToolBlocks(blocks []chatBlock) []chatBlock {
	if len(blocks) < 2 {
		return blocks
	}

	callByToolID := make(map[string]struct{}, len(blocks))
	resultByToolID := make(map[string]chatBlock, len(blocks))
	for i := range blocks {
		block := blocks[i]
		switch {
		case block.kind == blockToolCall && block.toolID != "":
			callByToolID[block.toolID] = struct{}{}
		case block.kind == blockToolResult && block.toolID != "":
			resultByToolID[block.toolID] = block
		}
	}

	merged := make([]chatBlock, 0, len(blocks))
	for i := range blocks {
		block := blocks[i]
		switch {
		case block.kind == blockToolCall && block.toolID != "":
			if result, ok := resultByToolID[block.toolID]; ok {
				toolName := block.toolName
				if toolName == "" {
					toolName = result.toolName
				}
				result.kind = blockToolResult
				result.toolName = toolName
				result.toolID = block.toolID
				result.args = block.args
				merged = append(merged, result)
				continue
			}
		case block.kind == blockToolResult && block.toolID != "":
			if _, ok := callByToolID[block.toolID]; ok {
				continue
			}
		}
		merged = append(merged, block)
	}

	if len(merged) < 2 {
		return merged
	}

	fallbackMerged := make([]chatBlock, 0, len(merged))
	for i := 0; i < len(merged); i++ {
		block := merged[i]
		if i+1 < len(merged) {
			next := merged[i+1]
			if block.kind == blockToolCall && block.toolID == "" &&
				next.kind == blockToolResult && next.toolID == "" &&
				block.toolName == next.toolName {
				result := next
				toolName := block.toolName
				if toolName == "" {
					toolName = result.toolName
				}
				result.kind = blockToolResult
				result.toolName = toolName
				result.args = block.args
				fallbackMerged = append(fallbackMerged, result)
				i++
				continue
			}
		}
		fallbackMerged = append(fallbackMerged, block)
	}

	return fallbackMerged
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
			return renderToolCall(styles, codersdk.ChatMessagePart{ToolName: block.toolName, Args: json.RawMessage(block.args)}, width)
		}
		return renderExpandedToolBlock(styles, styles.toolPending, "⏳", block.toolName, block.args, "", width)
	case blockToolResult:
		if !expanded {
			return renderToolResult(styles, codersdk.ChatMessagePart{ToolName: block.toolName, Args: json.RawMessage(block.args), Result: json.RawMessage(block.result), IsError: block.isError}, width)
		}
		icon := "✓"
		labelStyle := styles.toolSuccess
		if block.isError {
			icon = "✗"
			labelStyle = styles.errorText
		}
		result := block.result
		if strings.TrimSpace(result) == "" {
			result = "null"
		}
		return renderExpandedToolBlock(styles, labelStyle, icon, block.toolName, block.args, result, width)
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
