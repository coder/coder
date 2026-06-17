package chattool

import (
	"strings"

	"github.com/coder/coder/v2/coderd/render"
	coderstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// readmeInputMaxBytes caps how many README bytes are parsed so a giant README
// can't OOM coderd. It sits well above the output rune cap, so it never trims a
// real excerpt.
const readmeInputMaxBytes = 64 * 1024

// readmeText returns the README as bounded, frontmatter-stripped plain text
// truncated to maxRunes, or "" when the README is blank or conversion fails.
func readmeText(readme string, maxRunes int) string {
	// Cap the parse input first (see readmeInputMaxBytes); goldmark and the
	// tokenizer tolerate a mid-line or mid-rune cut.
	bounded := readme[:min(len(readme), readmeInputMaxBytes)]
	text, err := render.InnerTextFromMarkdown(stripReadmeFrontmatter(bounded))
	if err != nil {
		return ""
	}
	return coderstrings.Truncate(text, maxRunes, coderstrings.TruncateWithEllipsis)
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
