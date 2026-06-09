package chattool

import (
	"strings"

	coderstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// readmeExcerpt produces the bounded routing context surfaced in
// list_templates. It strips a leading frontmatter block so the excerpt budget
// is spent on prose rather than metadata, trims surrounding whitespace, and
// truncates to ListTemplatesReadmeExcerptMaxRunes with a trailing ellipsis so
// the agent can tell a clipped excerpt from a complete one. It returns the
// empty string when nothing remains.
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
// returned unchanged.
func stripReadmeFrontmatter(readme string) string {
	// Strip a leading UTF-8 BOM so a fence on the first line still matches.
	s := strings.TrimPrefix(readme, "\ufeff")
	lines := strings.Split(s, "\n")

	// Skip leading blank lines; the fence must be the first content.
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || !isFenceLine(lines[i]) {
		return readme
	}

	// Find the closing fence. Without one, there is no frontmatter to strip.
	for j := i + 1; j < len(lines); j++ {
		if isFenceLine(lines[j]) {
			return strings.Join(lines[j+1:], "\n")
		}
	}
	return readme
}

// isFenceLine reports whether a line is a "---" frontmatter fence, tolerating
// trailing whitespace and a carriage return from CRLF readmes.
func isFenceLine(line string) bool {
	return strings.TrimRight(line, " \t\r") == "---"
}
