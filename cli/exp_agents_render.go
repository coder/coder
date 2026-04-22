package cli

import (
	"bytes"
	"cmp"
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
	pendingToolIcon           = "○"
	reasoningPrefix           = "thinking: "
)

func compactTranscriptJSON(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}

	var builder bytes.Buffer
	if err := json.Compact(&builder, raw); err == nil {
		return builder.String()
	}

	return string(raw)
}

func toolBaseName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "coder_")
	name = strings.TrimPrefix(name, "github__")
	return strings.Join(strings.Fields(name), " ")
}

func humanizeToolName(name string) string {
	name = strings.ReplaceAll(toolBaseName(name), "_", " ")
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "tool"
	}
	return name
}

func normalizeToolName(name string) string {
	if toolBaseName(name) == "" {
		return ""
	}
	return strings.ReplaceAll(strings.ToLower(humanizeToolName(name)), " ", "_")
}

func summarizeToolContent(toolName, raw string, fields ...string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		if summary := toolObjectSummary(toolName, parsed); summary != "" {
			return summary
		}
		if value := firstStringField(parsed, fields...); value != "" {
			return strconv.Quote(value)
		}
		if value := firstShortStringValue(parsed); value != "" {
			return strconv.Quote(value)
		}
	}
	compact := compactTranscriptJSON(json.RawMessage(raw))
	if compact == "" {
		return ""
	}
	compactRunes := []rune(compact)
	if len(compactRunes) <= toolSummaryFallbackWidth {
		return compact
	}
	return string(compactRunes[:toolSummaryFallbackWidth-1]) + "…"
}

var toolArgsSummary = summarizeToolContent

func toolResultSummary(toolName, argsJSON, resultJSON string) string {
	return cmp.Or(
		summarizeToolContent(toolName, argsJSON),
		summarizeToolContent(toolName, resultJSON),
		"null",
	)
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

func toolDisplayLabel(toolName string, kind chatBlockKind, collapsedCount int) string {
	label := humanizeToolName(toolName)
	if collapsedCount <= 1 {
		return label
	}

	switch kind {
	case blockToolCall:
		return label + "..."
	case blockToolResult:
		return fmt.Sprintf("%s (x%d)", label, collapsedCount)
	default:
		return label
	}
}

func renderToolLine(styles tuiStyles, labelStyle lipgloss.Style, icon, label, summary string, width int) string {
	label = sanitizeTerminalRenderableText(label)
	summary = sanitizeTerminalRenderableText(summary)
	header := toolBlockIndent + labelStyle.Render(icon) + " " + label
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
	value = sanitizeTerminalRenderableText(value)
	if strings.TrimSpace(value) == "" {
		return ""
	}
	prefix := toolDetailIndent + label + ": "
	wrapped := wrapPreservingNewlines(value, contentWidth(width, lipgloss.Width(prefix)))
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
	if resultLine := renderToolDetail(styles, "result", result, width); resultLine != "" {
		lines = append(lines, resultLine)
	}
	return strings.Join(lines, "\n")
}

func toolResultIconAndStyle(styles tuiStyles, block chatBlock) (string, lipgloss.Style) {
	if block.isError {
		return "✗", styles.errorText
	}
	return "✓", styles.toolSuccess
}

func renderToolCallBlock(styles tuiStyles, block chatBlock, width int) string {
	if block.toolName == contextCompactionToolName {
		return renderCompaction(styles, width)
	}

	return renderToolLine(
		styles,
		styles.toolPending,
		pendingToolIcon,
		toolDisplayLabel(block.toolName, block.kind, block.collapsedCount),
		summarizeToolContent(block.toolName, block.args),
		width,
	)
}

func renderToolResultBlock(styles tuiStyles, block chatBlock, width int) string {
	if block.toolName == contextCompactionToolName {
		return renderCompaction(styles, width)
	}
	icon, labelStyle := toolResultIconAndStyle(styles, block)

	summary := summarizeToolContent(block.toolName, block.args)
	if summary == "" && block.isError {
		summary = summarizeToolContent("", block.result, "error", "message", "detail", "stderr")
	}
	if summary == "" {
		summary = toolResultSummary(block.toolName, "", block.result)
	}
	return renderToolLine(
		styles,
		labelStyle,
		icon,
		toolDisplayLabel(block.toolName, block.kind, block.collapsedCount),
		summary,
		width,
	)
}

func renderCompaction(styles tuiStyles, width int) string {
	banner := styles.compaction.Render("🗜️  Context compacted")
	if width <= 0 {
		return banner
	}
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, banner)
}

func contentWidth(width, inset int) int {
	if width <= 0 {
		return 80
	}
	return max(width-inset, 1)
}

func renderOverlayFrame(styles tuiStyles, width int, sections ...string) string {
	sections = slices.DeleteFunc(sections, func(section string) bool { return section == "" })
	return styles.overlayBorder.Width(contentWidth(width, 6)).Render(strings.Join(sections, "\n\n"))
}

func diffMetadataLines(diff codersdk.ChatDiffContents) []string {
	var lines []string
	if diff.Branch != nil && *diff.Branch != "" {
		lines = append(lines, fmt.Sprintf("Branch: %s", *diff.Branch))
	}
	if diff.PullRequestURL != nil && *diff.PullRequestURL != "" {
		lines = append(lines, fmt.Sprintf("PR: %s", *diff.PullRequestURL))
	}
	return lines
}

func parseChatGitChangesFromUnifiedDiff(diff codersdk.ChatDiffContents) []codersdk.ChatGitChange {
	rawDiff := sanitizeTerminalRenderableText(diff.Diff)
	if strings.TrimSpace(rawDiff) == "" {
		return nil
	}

	var (
		changes          []codersdk.ChatGitChange
		current          *codersdk.ChatGitChange
		currentAdditions int
		currentDeletions int
		inHunk           bool
	)
	flush := func() {
		if current == nil {
			return
		}
		if current.FilePath == "" {
			current = nil
			currentAdditions = 0
			currentDeletions = 0
			return
		}
		if currentAdditions > 0 || currentDeletions > 0 {
			stats := make([]string, 0, 2)
			if currentAdditions > 0 {
				stats = append(stats, fmt.Sprintf("+%d", currentAdditions))
			}
			if currentDeletions > 0 {
				stats = append(stats, fmt.Sprintf("-%d", currentDeletions))
			}
			summary := strings.Join(stats, " ")
			current.DiffSummary = &summary
		}
		changes = append(changes, *current)
		current = nil
		currentAdditions = 0
		currentDeletions = 0
	}

	for line := range strings.SplitSeq(rawDiff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			inHunk = false
			// parseUnifiedDiffHeaderPaths may return ("", "", false) when
			// the unquoted header form is ambiguous, such as a rename with
			// spaces in the paths. We still want to start a new entry so
			// the follow-up rename from / rename to / --- / +++ lines can
			// populate the correct paths. flush() drops entries that never
			// received a FilePath.
			oldPath, newPath, _ := parseUnifiedDiffHeaderPaths(line)
			current = &codersdk.ChatGitChange{
				ChatID:     diff.ChatID,
				FilePath:   newPath,
				ChangeType: "modified",
			}
			if oldPath != "" && newPath != "" && oldPath != newPath {
				oldPathCopy := oldPath
				current.OldPath = &oldPathCopy
				current.ChangeType = "renamed"
			}
		case current == nil:
			continue
		case strings.HasPrefix(line, "@@"):
			// Entering a hunk. Everything from here until the next
			// "diff --git " header is diff content, including any
			// added/removed lines that happen to start with "--- "
			// or "+++ ". Those must no longer be treated as file
			// headers.
			inHunk = true
		case !inHunk && strings.HasPrefix(line, "new file mode "):
			current.ChangeType = "added"
		case !inHunk && strings.HasPrefix(line, "deleted file mode "):
			current.ChangeType = "deleted"
		case !inHunk && strings.HasPrefix(line, "rename from "):
			// rename from/rename to paths are repository-relative and
			// never carry the a/ or b/ prefix, so we must not strip
			// those segments: a real file at a/foo.txt would otherwise
			// be truncated to foo.txt.
			oldPath := decodeQuotedDiffLinePath(strings.TrimPrefix(line, "rename from "))
			if oldPath != "" {
				oldPathCopy := oldPath
				current.OldPath = &oldPathCopy
			}
			current.ChangeType = "renamed"
		case !inHunk && strings.HasPrefix(line, "rename to "):
			newPath := decodeQuotedDiffLinePath(strings.TrimPrefix(line, "rename to "))
			if newPath != "" {
				current.FilePath = newPath
			}
			current.ChangeType = "renamed"
		case !inHunk && strings.HasPrefix(line, "--- /dev/null"):
			current.ChangeType = "added"
		case !inHunk && strings.HasPrefix(line, "+++ /dev/null"):
			current.ChangeType = "deleted"
		case !inHunk && strings.HasPrefix(line, "--- "):
			if current.ChangeType == "added" {
				continue
			}
			if oldPath := trimUnifiedDiffPath(strings.TrimPrefix(line, "--- ")); oldPath != "" && oldPath != "/dev/null" {
				oldPathCopy := oldPath
				current.OldPath = &oldPathCopy
			}
		case !inHunk && strings.HasPrefix(line, "+++ "):
			if current.ChangeType == "deleted" {
				continue
			}
			if newPath := trimUnifiedDiffPath(strings.TrimPrefix(line, "+++ ")); newPath != "" && newPath != "/dev/null" {
				current.FilePath = newPath
			}
		case inHunk && strings.HasPrefix(line, "+"):
			currentAdditions++
		case inHunk && strings.HasPrefix(line, "-"):
			currentDeletions++
		}
	}
	flush()
	return changes
}

// parseUnifiedDiffHeaderPaths extracts the old and new paths from a
// `diff --git ...` header line. Git emits paths in one of two forms:
//
//  1. Quoted: `diff --git "a/<old>" "b/<new>"`. Used when paths contain
//     control characters, backslashes, double quotes, or (with the default
//     core.quotepath setting) bytes above 0x7f. The contents are C-quoted.
//  2. Unquoted: `diff --git a/<old> b/<new>`. Used for simple paths, which
//     may still contain spaces. Because there is no delimiter between the
//     two paths, this form is ambiguous when paths contain spaces: we rely
//     on the git convention that non-rename diffs repeat the same path in
//     both halves.
//
// For the unquoted form we first search for a split point at ` b/` where
// the left and right halves are equal after stripping the `a/` and `b/`
// prefixes (the non-rename case). If that fails but the line contains only
// a single space, we split there for simple renames with no embedded
// whitespace. Otherwise we return ok=false and let the caller rely on the
// subsequent `rename from`, `rename to`, `--- `, and `+++ ` lines.
func parseUnifiedDiffHeaderPaths(line string) (oldPath string, newPath string, ok bool) {
	raw := strings.TrimSpace(strings.TrimPrefix(line, "diff --git "))
	if raw == "" {
		return "", "", false
	}

	if strings.HasPrefix(raw, `"`) {
		old, rest, ok := consumeQuotedDiffPath(raw)
		if !ok {
			return "", "", false
		}
		rest = strings.TrimLeft(rest, " ")
		newp, _, ok := consumeQuotedDiffPath(rest)
		if !ok {
			return "", "", false
		}
		// The unquoted values already have their surrounding quotes removed,
		// so we must not feed them to trimUnifiedDiffPath (which would strip
		// any legitimate leading or trailing quote characters in the file
		// name). Only strip the a/ or b/ prefix here.
		return stripUnifiedDiffPrefix(old), stripUnifiedDiffPrefix(newp), true
	}

	if !strings.HasPrefix(raw, "a/") {
		return "", "", false
	}
	for offset := 0; offset < len(raw); {
		idx := strings.Index(raw[offset:], " b/")
		if idx < 0 {
			break
		}
		pos := offset + idx
		left := trimUnifiedDiffPath(raw[:pos])
		right := trimUnifiedDiffPath(raw[pos+1:])
		if left == right {
			return left, right, true
		}
		offset = pos + 1
	}
	// No equal split was found. If the line only contains a single space,
	// the split is unambiguous and this is a simple rename whose paths
	// happen to differ. Splitting the quoted-path form was handled above,
	// so we know the raw form has no quoting to worry about here.
	if strings.Count(raw, " ") == 1 {
		idx := strings.Index(raw, " b/")
		if idx > 0 {
			return trimUnifiedDiffPath(raw[:idx]), trimUnifiedDiffPath(raw[idx+1:]), true
		}
	}
	return "", "", false
}

// consumeQuotedDiffPath reads one C-quoted path from the start of s and
// returns the unquoted value along with the remainder of the string. The
// leading character of s must be `"`. git's C-quoting matches Go's quoted
// string syntax closely enough for strconv.Unquote to handle the common
// cases (octal byte escapes like `\303`, and the usual `\t`, `\n`, `\"`,
// `\\`).
func consumeQuotedDiffPath(s string) (path string, rest string, ok bool) {
	if !strings.HasPrefix(s, `"`) {
		return "", "", false
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			// Skip the next byte so an escaped quote does not terminate
			// the literal early. Bounds-check to avoid running off the
			// end of a malformed input.
			if i+1 >= len(s) {
				return "", "", false
			}
			i++
		case '"':
			unq, err := strconv.Unquote(s[:i+1])
			if err != nil {
				return "", "", false
			}
			return unq, s[i+1:], true
		}
	}
	return "", "", false
}

// trimUnifiedDiffPath decodes a path taken from a `--- ` or `+++ ` line
// of a unified diff. Those lines always prefix the path with `a/` or `b/`,
// so the prefix is stripped after any C-quote decoding.
func trimUnifiedDiffPath(path string) string {
	return stripUnifiedDiffPrefix(decodeQuotedDiffLinePath(path))
}

// decodeQuotedDiffLinePath decodes a git-emitted path without stripping
// any `a/` or `b/` prefix. Git only adds those prefixes to `diff --git`,
// `--- `, and `+++ ` lines, so `rename from`, `rename to`, and similar
// lines must use this helper to avoid truncating a real leading `a/` or
// `b/` directory component.
func decodeQuotedDiffLinePath(path string) string {
	path = strings.TrimSpace(path)
	// Git quotes the whole path with double quotes and C-style escapes when
	// it contains control characters, backslashes, double quotes, or (with
	// the default core.quotepath setting) bytes above 0x7f. strconv.Unquote
	// understands the same escape vocabulary for the common cases.
	if len(path) >= 2 && strings.HasPrefix(path, `"`) && strings.HasSuffix(path, `"`) {
		if unq, err := strconv.Unquote(path); err == nil {
			return unq
		}
		return strings.Trim(path, `"`)
	}
	return path
}

func stripUnifiedDiffPrefix(path string) string {
	switch {
	case strings.HasPrefix(path, "a/"), strings.HasPrefix(path, "b/"):
		return path[2:]
	default:
		return path
	}
}

// agentgitOversizePlaceholderPrefix matches the literal prefix that
// agent/agentgit substitutes for a repository's UnifiedDiff when the
// raw diff exceeds maxTotalDiffSize (3 MiB). See
// agent/agentgit/agentgit.go. Multi-repo aggregates assembled by
// buildLocalChatDiffContents can mix real `diff --git` chunks with
// this placeholder, in which case parseChatGitChangesFromUnifiedDiff
// returns a non-zero count for the real chunks while silently
// dropping the placeholder repo. Detecting the prefix separately
// lets renderChatDiffSummary flag the omission so the user is not
// misled into thinking the summary is exhaustive. Kept as a local
// prefix match because the coupling is narrow and the string is
// stable.
const agentgitOversizePlaceholderPrefix = "Total diff too large to show. Size:"

// hasOversizedRepoPlaceholder reports whether the combined unified
// diff contains at least one agentgit oversize-repo placeholder.
// Matching is scoped to lines that start with the placeholder prefix
// so a false positive from a diff body that legitimately contains the
// phrase (e.g. as a `+` added line inside a real patch) cannot
// trigger the omission notice. agentgit always writes the
// placeholder as the entire UnifiedDiff for a repo, and
// buildLocalChatDiffContents joins segments with "\n", so a real
// placeholder repo always appears on its own line after the join.
func hasOversizedRepoPlaceholder(diff string) bool {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, agentgitOversizePlaceholderPrefix) {
			return true
		}
	}
	return false
}

func renderChatDiffSummary(diff codersdk.ChatDiffContents) string {
	changes := parseChatGitChangesFromUnifiedDiff(diff)
	if len(changes) == 0 {
		// The diff text might be non-empty but not in `diff --git`
		// format (for example `agent/agentgit` emits a "Total diff
		// too large to show..." placeholder when the raw diff exceeds
		// the read limit). Report that changes exist but could not
		// be summarized so we do not mislead the user into thinking
		// the workspace is clean.
		if strings.TrimSpace(diff.Diff) != "" {
			return "Changes present but could not be summarized."
		}
		return "No changes detected."
	}

	label := "files"
	if len(changes) == 1 {
		label = "file"
	}
	lines := []string{fmt.Sprintf("%d %s changed:", len(changes), label)}
	for _, change := range changes {
		path := sanitizeTerminalRenderableText(change.FilePath)
		if change.ChangeType == "renamed" && change.OldPath != nil && *change.OldPath != "" {
			path = fmt.Sprintf("%s → %s", sanitizeTerminalRenderableText(*change.OldPath), path)
		}
		line := fmt.Sprintf("  %-8s %s", change.ChangeType, path)
		if change.DiffSummary != nil && strings.TrimSpace(*change.DiffSummary) != "" {
			line = fmt.Sprintf("%s (%s)", line, sanitizeTerminalRenderableText(*change.DiffSummary))
		}
		lines = append(lines, line)
	}
	// A multi-repo aggregate can mix real diff chunks (counted
	// above) with agentgit's oversize placeholder for repos whose
	// raw diff exceeds maxTotalDiffSize. The placeholder does not
	// contribute to the files-changed count because it is not in
	// `diff --git` format, so without this notice the summary would
	// silently underreport the changeset.
	if hasOversizedRepoPlaceholder(diff.Diff) {
		lines = append(lines, "  (some repositories omitted: diff too large to summarize)")
	}
	return strings.Join(lines, "\n")
}

func renderStyledDiffBody(styles tuiStyles, diff string) string {
	diff = sanitizeTerminalRenderableText(diff)
	if strings.TrimSpace(diff) == "" {
		return styles.dimmedText.Render("No diff contents.")
	}
	lines := strings.Split(diff, "\n")
	inHunk := false
	for i, line := range lines {
		// Track whether we're inside a hunk body so styling can
		// distinguish legitimate header `--- `/`+++ ` lines from
		// additions/deletions whose content happens to start with
		// those prefixes (for example a `+++ ` content line whose
		// text begins with `++ `). Matches the parser's inHunk
		// bookkeeping in parseChatGitChangesFromUnifiedDiff.
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inHunk = false
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		}
		lines[i] = styleUnifiedDiffLine(styles, line, inHunk)
	}
	return strings.Join(lines, "\n")
}

func styleUnifiedDiffLine(styles tuiStyles, line string, inHunk bool) string {
	switch {
	case strings.HasPrefix(line, "diff --git "):
		return styles.selectedItem.Render(line)
	case strings.HasPrefix(line, "index "),
		strings.HasPrefix(line, "new file mode "),
		strings.HasPrefix(line, "deleted file mode "),
		strings.HasPrefix(line, "rename from "),
		strings.HasPrefix(line, "rename to "),
		strings.HasPrefix(line, "Binary files "):
		return styles.subtitle.Render(line)
	case !inHunk && (strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ")):
		return styles.subtitle.Render(line)
	case strings.HasPrefix(line, "@@"):
		return styles.warningText.Render(line)
	case strings.HasPrefix(line, "+"):
		return styles.toolSuccess.Render(line)
	case strings.HasPrefix(line, "-"):
		return styles.errorText.Render(line)
	default:
		return line
	}
}

// renderDiffDrawer builds the diff overlay contents. The caller is
// responsible for producing summary with renderChatDiffSummary and
// styledBody with renderStyledDiffBody so that every View() redraw
// does not walk the full (potentially 4 MiB) diff through
// parseChatGitChangesFromUnifiedDiff or re-style every line through
// lipgloss. chatViewModel caches both in diffSummary and
// diffStyledBody for this reason. If styledBody is empty the caller
// had no cache (for example tests that construct diffs directly), so
// fall back to computing it here instead of silently rendering an
// empty body.
func renderDiffDrawer(styles tuiStyles, diff codersdk.ChatDiffContents, summary, styledBody string, width, height int) string {
	innerWidth := contentWidth(width, 6)
	headerBits := []string{styles.title.Render("Diff")}
	if meta := diffMetadataLines(diff); len(meta) > 0 {
		headerBits = append(headerBits, styles.subtitle.Render(strings.Join(meta, " • ")))
	}
	diffBody := styledBody
	if diffBody == "" {
		diffBody = renderStyledDiffBody(styles, diff.Diff)
	}
	help := styles.helpText.Render("Esc to close")
	overhead := countRenderedLines(strings.Join(headerBits, "\n")) + countRenderedLines(summary) + countRenderedLines(help) + 4
	availableBodyLines := max(height-overhead, 0)
	if height <= 0 {
		availableBodyLines = 12
	}
	wrappedDiff := wrapPreservingNewlines(diffBody, innerWidth)
	if availableBodyLines == 0 {
		wrappedDiff = ""
	} else {
		wrappedDiff = clampLines(wrappedDiff, availableBodyLines)
	}
	return renderOverlayFrame(styles, width, strings.Join(headerBits, "\n"), summary, wrappedDiff, help)
}

func renderModelPicker(styles tuiStyles, catalog codersdk.ChatModelsResponse, selected string, cursor int, width, height int) string {
	innerWidth := contentWidth(width, 6)
	lines := []string{styles.title.Render("Select Model")}
	cursorLine := 0
	hasModels := false
	flatIndex := 0
	for _, provider := range catalog.Providers {
		if len(provider.Models) == 0 {
			continue
		}
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
			if flatIndex == cursor {
				cursorLine = len(lines) - 1
			}
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
	maxContentLines := max(height-countRenderedLines(help)-4, 1)
	if height <= 0 {
		maxContentLines = len(contentLines)
	}
	windowStart := 0
	if cursorLine >= maxContentLines {
		windowStart = cursorLine - maxContentLines + 1
	}
	maxWindowStart := max(len(contentLines)-maxContentLines, 0)
	windowStart = min(windowStart, maxWindowStart)
	windowEnd := min(windowStart+maxContentLines, len(contentLines))
	content := append([]string(nil), contentLines[windowStart:windowEnd]...)
	content = append(content, help)
	return renderOverlayFrame(styles, width, strings.Join(content, "\n"))
}

func renderAskUserQuestion(styles tuiStyles, state *askUserQuestionState, width, height int) string {
	if state == nil || len(state.Questions) == 0 {
		return ""
	}
	if state.CurrentIndex < 0 || state.CurrentIndex >= len(state.Questions) {
		return ""
	}

	innerWidth := contentWidth(width, 6)
	question := state.Questions[state.CurrentIndex]
	sections := []string{styles.title.Render(fmt.Sprintf("Plan Question %d/%d", state.CurrentIndex+1, len(state.Questions)))}
	if question.Header != "" {
		sections = append(sections, styles.subtitle.Render(sanitizeTerminalRenderableText(question.Header)))
	}
	sections = append(sections, wrapPreservingNewlines(sanitizeTerminalRenderableText(question.Question), innerWidth))

	if state.Submitting {
		sections = append(sections, styles.dimmedText.Render("Submitting answers..."))
		return renderOverlayFrame(styles, width, sections...)
	}

	optionLines := make([]string, 0, len(question.Options)+3)
	for i, option := range question.Options {
		label := strings.TrimSpace(sanitizeTerminalRenderableText(option.Label))
		if label == "" {
			label = "(empty option)"
		}
		label = styles.truncate(label, max(innerWidth-2, 0))
		row := "  " + label
		if i == state.OptionCursor {
			row = styles.selectedItem.Render("> " + label)
		}
		optionLines = append(optionLines, row)
	}

	otherLabel := styles.truncate("Other (type custom answer)", max(innerWidth-2, 0))
	otherRow := "  " + otherLabel
	if state.OptionCursor == len(question.Options) {
		otherRow = styles.selectedItem.Render("> " + otherLabel)
	}
	optionLines = append(optionLines, otherRow)
	if state.OtherMode {
		optionLines = append(optionLines, "", state.OtherInput.View())
	}
	sections = append(sections, strings.Join(optionLines, "\n"))

	if state.Error != nil {
		sections = append(sections, styles.errorText.Render(wrapPreservingNewlines(
			"Error: "+sanitizeTerminalRenderableText(state.Error.Error()),
			innerWidth,
		)))
	}

	longHelpParts := []string{"↑/↓ navigate", "enter select"}
	shortHelpParts := []string{"↑↓", "↵"}
	compactHelpParts := []string{"↑↓", "↵"}
	if state.CurrentIndex > 0 {
		longHelpParts = append(longHelpParts, "←/h back")
		shortHelpParts = append(shortHelpParts, "←/h")
		compactHelpParts = append(compactHelpParts, "←")
	}
	if state.OtherMode {
		longHelpParts = append(longHelpParts, "esc cancel input")
		shortHelpParts = append(shortHelpParts, "esc input")
		compactHelpParts = append(compactHelpParts, "esc")
	}
	sections = append(sections, styles.helpText.Render(fitHelpText(
		innerWidth,
		strings.Join(longHelpParts, " | "),
		strings.Join(shortHelpParts, " │ "),
		strings.Join(compactHelpParts, " "),
	)))

	_ = height
	return renderOverlayFrame(styles, width, sections...)
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
	activeSelection := -1
	if !composerFocused {
		activeSelection = selectedBlock
	}
	visibleIndices := collapseConsecutiveSameNameBlocks(blocks, activeSelection, expandedBlocks)
	rendered := make([]string, 0, len(visibleIndices))
	for _, index := range visibleIndices {
		blockView := blocks[index].cachedRender
		if blockView == "" ||
			blocks[index].cachedWidth != width ||
			blocks[index].cachedExpanded != expandedBlocks[index] ||
			blocks[index].cachedCollapsedCount != blocks[index].collapsedCount {
			blockView = renderBlock(styles, blocks[index], expandedBlocks[index], width, renderer)
			blocks[index].cachedRender = blockView
			blocks[index].cachedWidth = width
			blocks[index].cachedExpanded = expandedBlocks[index]
			blocks[index].cachedCollapsedCount = blocks[index].collapsedCount
		}
		if index == activeSelection {
			blockView = styles.selectedBlock.Render(blockView)
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

func collapseConsecutiveSameNameBlocks(blocks []chatBlock, selectedBlock int, expandedBlocks map[int]bool) []int {
	if len(blocks) == 0 {
		return nil
	}

	for i := range blocks {
		blocks[i].collapsedCount = 0
	}

	visibleIndices := make([]int, 0, len(blocks))
	for i := 0; i < len(blocks); {
		runEnd := i + 1
		for runEnd < len(blocks) && canCollapseToolBlocks(blocks[i], blocks[runEnd]) {
			runEnd++
		}

		if runEnd-i < 2 || hasExpandedToolBlock(expandedBlocks, i, runEnd) {
			for j := i; j < runEnd; j++ {
				visibleIndices = append(visibleIndices, j)
			}
			i = runEnd
			continue
		}

		representative := i
		if selectedBlock >= i && selectedBlock < runEnd {
			representative = selectedBlock
		}
		blocks[representative].collapsedCount = runEnd - i
		visibleIndices = append(visibleIndices, representative)
		i = runEnd
	}

	return visibleIndices
}

func canCollapseToolBlocks(a, b chatBlock) bool {
	if a.kind != b.kind {
		return false
	}
	if a.kind != blockToolCall && a.kind != blockToolResult {
		return false
	}
	if a.toolName != b.toolName {
		return false
	}
	if a.kind == blockToolResult && a.isError != b.isError {
		return false
	}
	if a.args != b.args || a.result != b.result {
		return false
	}
	return true
}

func hasExpandedToolBlock(expandedBlocks map[int]bool, start, end int) bool {
	for i := start; i < end; i++ {
		if expandedBlocks[i] {
			return true
		}
	}
	return false
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
			case codersdk.ChatMessagePartTypeToolCall, codersdk.ChatMessagePartTypeToolResult:
				block := chatBlock{role: message.Role, toolName: part.ToolName, toolID: part.ToolCallID}
				switch {
				case part.ToolName == contextCompactionToolName:
					block.kind = blockCompaction
				case part.Type == codersdk.ChatMessagePartTypeToolCall:
					block.kind = blockToolCall
					block.args = compactTranscriptJSON(part.Args)
				default:
					block.kind = blockToolResult
					block.result = compactTranscriptJSON(part.Result)
					block.isError = part.IsError
				}
				blocks = append(blocks, block)
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

func mergeToolResult(call, result chatBlock) chatBlock {
	if call.toolName != "" {
		result.toolName = call.toolName
	}
	result.kind = blockToolResult
	result.toolID = call.toolID
	result.args = call.args
	return result
}

func mergeConsecutiveToolBlocks(blocks []chatBlock) []chatBlock {
	if len(blocks) < 2 {
		return blocks
	}

	merged := make([]chatBlock, 0, len(blocks))
	for i := 0; i < len(blocks); i++ {
		block := blocks[i]
		if i+1 < len(blocks) {
			next := blocks[i+1]
			if block.kind == blockToolCall && next.kind == blockToolResult {
				switch {
				case block.toolID != "" && block.toolID == next.toolID:
					merged = append(merged, mergeToolResult(block, next))
					i++
					continue
				case block.toolID == "" && next.toolID == "" && block.toolName == next.toolName:
					merged = append(merged, mergeToolResult(block, next))
					i++
					continue
				}
			}
		}
		merged = append(merged, block)
	}
	return merged
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
			return styles.dimmedText.Render(wrapPreservingNewlines(sanitizeTerminalRenderableText(block.text), width))
		default:
			return wrapPreservingNewlines(sanitizeTerminalRenderableText(block.text), width)
		}
	case blockReasoning:
		content := wrapPreservingNewlines(reasoningPrefix+sanitizeTerminalRenderableText(block.text), width)
		if !expanded {
			content = clampLines(content, 3)
		}
		return styles.reasoning.Render(content)
	case blockToolCall:
		if !expanded {
			return renderToolCallBlock(styles, block, width)
		}
		return renderExpandedToolBlock(styles, styles.toolPending, pendingToolIcon, block.toolName, block.args, "", width)
	case blockToolResult:
		if !expanded {
			return renderToolResultBlock(styles, block, width)
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

var (
	fallbackMarkdownRenderers sync.Map
	markdownRendererMu        sync.Mutex
)

func getFallbackMarkdownRenderer(width int) *glamour.TermRenderer {
	wrapWidth := contentWidth(width, 0)
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
	text = sanitizeTerminalRenderableText(text)
	var renderer *glamour.TermRenderer
	if len(renderers) > 0 {
		renderer = renderers[0]
	}
	if renderer == nil {
		renderer = getFallbackMarkdownRenderer(width)
	}
	if renderer != nil {
		markdownRendererMu.Lock()
		rendered, err := renderer.Render(text)
		markdownRendererMu.Unlock()
		if err == nil {
			trimmedRendered := strings.TrimRight(rendered, "\n")
			if strings.TrimSpace(trimmedRendered) != "" || strings.TrimSpace(text) == "" {
				return styles.assistantMsg.Render(trimmedRendered)
			}
		}
	}
	return styles.assistantMsg.Render(wrapPreservingNewlines(text, width))
}

func renderPrefixedBlock(prefix, body string, width int) string {
	body = sanitizeTerminalRenderableText(body)
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
	return strings.Join(clampLineSlice(strings.Split(text, "\n"), maxLines), "\n")
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
