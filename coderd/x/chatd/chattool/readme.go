package chattool

import (
	"strings"

	coderstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// readmeExcerpt produces the bounded routing context for list_templates.
// Frontmatter is stripped so prose, not metadata, fills the budget, and the
// ellipsis lets the agent distinguish a clipped excerpt from a complete one.
func readmeExcerpt(readme string) string {
	body := strings.TrimSpace(stripReadmeFrontmatter(readme))
	if body == "" {
		return ""
	}
	return coderstrings.Truncate(body, ListTemplatesReadmeExcerptMaxRunes, coderstrings.TruncateWithEllipsis)
}

// stripReadmeFrontmatter removes a single leading "---" fenced YAML
// frontmatter block so README routing prose, not metadata, fills the excerpt
// budget. READMEs without a leading fence, or with an unterminated fence, are
// returned with only a leading BOM stripped. A column-0 "---" inside a quoted
// multiline scalar can be mistaken for the closing fence; this is accepted to
// avoid a full YAML parser.
func stripReadmeFrontmatter(readme string) string {
	// Strip a leading UTF-8 BOM so a fence on the first line still matches, and
	// so the BOM never leaks into the excerpt on the unchanged paths below.
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
