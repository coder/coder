package chattool

import (
	"bufio"
	"strings"
	"unicode"

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

// stripReadmeFrontmatter strips a leading frontmatter block from the README if it
// exists. An unterminated frontmatter block is treated as a regular body section.
// UTF-8 BOMs are stripped if present, and CRLF is normalized to LF. Leading
// whitespace is also stripped.
func stripReadmeFrontmatter(readme string) string {
	trimmed := strings.TrimLeftFunc(readme, func(r rune) bool {
		return unicode.IsSpace(r) || r == '\ufeff'
	})

	var out strings.Builder
	scn := bufio.NewScanner(strings.NewReader(trimmed))
	scn.Buffer(nil, readmeInputMaxBytes+1) // headroom
	var lineNumber int
	var fences int
	for scn.Scan() {
		line := scn.Text()
		lineNumber++

		// Only handle fences if we haven't already found an
		// opening and closing fence.
		if fences < 2 {
			isFence := strings.TrimRight(line, " \t\r") == "---"
			if isFence {
				fences++
				continue
			} else if lineNumber == 1 {
				// No leading fence -> no frontmatter. Return the entire document.
				return trimmed
			}
			// We are still in the frontmatter block. Skip writing this line.
			continue
		}
		_, _ = out.WriteString(line)
		_, _ = out.WriteString("\n")
	}

	// Can err if input is too big. Shouldn't happen normally.
	if scn.Err() != nil {
		return trimmed
	}

	// Scanner did not scan any lines. No frontmatter to strip.
	if lineNumber == 0 {
		return trimmed
	}

	// If we are still fenced but we reached the end of the document
	// we have an unterminated fence.
	if fences == 1 {
		return trimmed
	}
	return out.String()
}
