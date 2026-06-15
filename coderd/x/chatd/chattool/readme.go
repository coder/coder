package chattool

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	coderstrings "github.com/coder/coder/v2/coderd/util/strings"
)

// readmeParser parses README markdown for prose extraction. The Table extension
// is enabled so tables parse as table nodes we can drop, rather than as
// paragraphs of pipe-delimited text. It is read-only after construction and
// safe for concurrent use.
var readmeParser = goldmark.New(goldmark.WithExtensions(extension.Table)).Parser()

// readmeExcerpt produces the bounded routing context for list_templates.
// The README is reduced to plain-text prose (see extractReadmeProse) and
// truncated; the ellipsis lets the agent distinguish a clipped excerpt from a
// complete one.
func readmeExcerpt(readme string) string {
	prose := extractReadmeProse(stripReadmeFrontmatter(readme))
	if prose == "" {
		return ""
	}
	return coderstrings.Truncate(prose, ListTemplatesReadmeExcerptMaxRunes, coderstrings.TruncateWithEllipsis)
}

// extractReadmeProse parses README markdown and returns its prose as a single
// space-joined plain-text string. Blocks that waste the budget without aiding
// template selection are dropped entirely: images, HTML blocks and inline HTML,
// code blocks, tables, and thematic breaks. Inline links keep their visible text
// but drop the URL (URLs live on the link node, not in its text). Returns "" if
// no prose remains.
func extractReadmeProse(body string) string {
	src := []byte(body)
	doc := readmeParser.Parse(text.NewReader(src))

	var b strings.Builder
	// Walk errors only ever originate from the walker; ours never returns one.
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindImage, ast.KindRawHTML, ast.KindHTMLBlock,
			ast.KindCodeBlock, ast.KindFencedCodeBlock, ast.KindThematicBreak,
			extast.KindTable:
			// Drop the node and its subtree (e.g. image alt text, table cells).
			return ast.WalkSkipChildren, nil
		case ast.KindHeading, ast.KindParagraph, ast.KindTextBlock:
			// Separate consecutive text blocks (heading/paragraph/list item) with
			// a space. Spacing *within* a block comes from the source segments.
			if b.Len() > 0 {
				_ = b.WriteByte(' ')
			}
		case ast.KindText:
			if t, ok := n.(*ast.Text); ok {
				_, _ = b.Write(t.Segment.Value(src))
				// A soft/hard line break inside a block is whitespace in the
				// source that the segment does not include; re-add it.
				if t.SoftLineBreak() || t.HardLineBreak() {
					_ = b.WriteByte(' ')
				}
			}
		}
		return ast.WalkContinue, nil
	})

	// Collapse all whitespace (including block boundaries) to single spaces.
	return strings.Join(strings.Fields(b.String()), " ")
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
