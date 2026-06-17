package chattool

import (
	"strings"

	"github.com/coder/coder/v2/coderd/render"
	coderstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// version.Readme is capped only by the ~100MB tarball limit, so bound it to
// 64KiB to avoid pathological inputs.
const readmeInputMaxBytes = 64 * 1024

// readmeText returns the README as bounded, frontmatter-stripped plain text
// truncated to maxRunes, or "" when the README is blank or conversion fails.
func readmeText(readme string, maxRunes int) string {
	body := stripReadmeFrontmatter(boundInput(readme))
	text, err := render.InnerTextFromMarkdown(body)
	if err != nil {
		return ""
	}
	return coderstrings.Truncate(text, maxRunes, coderstrings.TruncateWithEllipsis)
}

// boundInput caps s at readmeInputMaxBytes, backing off to the last newline so
// the parser never sees a line cut mid-tag. A capped run with no newline is
// hard-cut.
func boundInput(s string) string {
	if len(s) <= readmeInputMaxBytes {
		return s
	}
	if i := strings.LastIndexByte(s[:readmeInputMaxBytes], '\n'); i >= 0 {
		return s[:i]
	}
	return s[:readmeInputMaxBytes]
}

// stripReadmeFrontmatter removes a single leading "---" YAML frontmatter block
// so routing prose, not metadata, fills the budget. Without a leading or closing
// fence, only a leading BOM is stripped. A "---" inside a quoted YAML scalar can
// be misread as the closing fence; accepted to avoid a full YAML parser.
func stripReadmeFrontmatter(readme string) string {
	// Strip a leading UTF-8 BOM so the first-line fence matches and the BOM
	// never leaks downstream.
	s := strings.TrimPrefix(readme, "\ufeff")
	lines := strings.Split(s, "\n")

	// Skip leading blank lines; the fence must be the first content.
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || !isFenceLine(lines[i]) {
		return s
	}

	// Find the closing fence. Without one, there is no frontmatter to strip.
	for j := i + 1; j < len(lines); j++ {
		if isFenceLine(lines[j]) {
			return strings.Join(lines[j+1:], "\n")
		}
	}
	return s
}

// isFenceLine reports whether a line is a "---" frontmatter fence, tolerating
// trailing whitespace and a carriage return from CRLF readmes.
func isFenceLine(line string) bool {
	return strings.TrimRight(line, " \t\r") == "---"
}
