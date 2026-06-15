package chattool

import (
	"strings"

	"github.com/coder/coder/v2/coderd/render"
	coderstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// readmeInputMaxBytes bounds how much README markdown is parsed. version.Readme
// is only capped by the ~100MB tarball limit, so without this a pathological
// README would build an HTML tree far larger than any excerpt needs. 64KiB sits
// well above the largest (8192-rune) output cap.
const readmeInputMaxBytes = 64 * 1024

// readmeText strips frontmatter, bounds the input, renders the README to its
// plain-text innerText, and truncates to maxRunes with an ellipsis. Returns ""
// when the README is empty/whitespace-only or conversion fails (best-effort):
// markdown formatting, images, and link URLs become their visible text while
// code blocks and table cells are preserved.
func readmeText(readme string, maxRunes int) string {
	body := stripReadmeFrontmatter(readme)
	// Back off to the last newline so the parser is not handed a fragment cut
	// mid-line (which would leak debris such as a half-written tag).
	if len(body) > readmeInputMaxBytes {
		if i := strings.LastIndexByte(body[:readmeInputMaxBytes], '\n'); i >= 0 {
			body = body[:i]
		} else {
			body = body[:readmeInputMaxBytes]
		}
	}

	text, err := render.InnerTextFromMarkdown(body)
	if err != nil {
		return ""
	}
	return coderstrings.Truncate(text, maxRunes, coderstrings.TruncateWithEllipsis)
}

// stripReadmeFrontmatter removes a single leading "---" fenced YAML
// frontmatter block so README routing prose, not metadata, fills the budget.
// READMEs without a leading fence, or with an unterminated fence, are returned
// with only a leading BOM stripped. A column-0 "---" inside a quoted multiline
// scalar can be mistaken for the closing fence; this is accepted to avoid a full
// YAML parser.
func stripReadmeFrontmatter(readme string) string {
	// Strip a leading UTF-8 BOM so a fence on the first line still matches, and
	// so the BOM never leaks into the output on the unchanged paths below.
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
