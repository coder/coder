package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"golang.org/x/net/html"
)

// kindEmptyTableHeader is a table header cell with no text. Screen readers
// announce such a column or row without a name, and axe reports it as
// empty-table-header.
const kindEmptyTableHeader findingKind = "empty-table-header"

// headerCell is one header cell of a table: the line it starts on and its
// text with whitespace collapsed.
type headerCell struct {
	line int
	text string
}

// checkTables returns a finding for every table header cell with no text, in
// both GFM pipe tables (the header row) and raw HTML tables (any <th>). Text is
// whatever a reader would hear: entities such as &nbsp; are decoded and
// whitespace is trimmed, so <th>&nbsp;</th> and <th><br></th> count as empty,
// while an image's alt text, an autolink, and code count as text. Tables
// inside code blocks and code spans are ignored.
func checkTables(src []byte) []finding {
	doc := goldmark.New(goldmark.WithExtensions(extension.Table)).Parser().Parse(text.NewReader(src))
	t := &tableChecker{c: checker{src: src, lineStarts: lineStarts(src)}}

	preorder(doc, func(n ast.Node) bool {
		switch node := n.(type) {
		case *east.TableHeader:
			t.pipeHeader(node)
			return false
		case *ast.HTMLBlock:
			segs := segmentsOf(node.Lines())
			if node.HasClosure() {
				segs = append(segs, node.ClosureLine)
			}
			t.scanHTML(segs)
		case *ast.RawHTML:
			t.scanHTML(segmentsOf(node.Segments))
		case *ast.Text:
			// Markdown between the raw-HTML blocks of an open <th>, such as
			// <th>\n\n**Feature**\n\n</th>, is that cell's text.
			t.addText(textValue(node, src))
		case *ast.String:
			t.addText(string(node.Value))
		case *ast.AutoLink:
			t.addText(string(node.Label(src)))
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			for _, seg := range segmentsOf(n.Lines()) {
				t.addText(string(seg.Value(src)))
			}
		}
		return true
	})

	// A table left unclosed at end of file still reports its empty headers.
	for len(t.tables) > 0 {
		t.closeTable()
	}

	slices.SortStableFunc(t.findings, func(a, b finding) int {
		return cmp.Compare(a.line, b.line)
	})
	return t.findings
}

type tableChecker struct {
	// c supplies the source and line lookup; its tag-balance state is unused.
	c        checker
	findings []finding
	// tables is the stack of open HTML tables (they can nest); each entry
	// holds that table's <th> cells so far.
	tables [][]headerCell
	// inTH reports whether the innermost table has an open <th>, whose text
	// accumulates in the last cell of that table.
	inTH bool
}

// pipeHeader checks the header row of a GFM pipe table.
func (t *tableChecker) pipeHeader(header *east.TableHeader) {
	line := t.c.lineAt(header.Pos())
	var cells []headerCell
	for cell := header.FirstChild(); cell != nil; cell = cell.NextSibling() {
		cells = append(cells, headerCell{line: line, text: collapse(inlineText(cell, t.c.src))})
	}
	t.report(cells)
}

// inlineText returns the text a reader would hear from a pipe table cell.
func inlineText(n ast.Node, src []byte) string {
	var b []byte
	preorder(n, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Text:
			b = append(b, textValue(node, src)...)
		case *ast.String:
			b = append(b, node.Value...)
		case *ast.AutoLink:
			b = append(b, node.Label(src)...)
		case *ast.RawHTML:
			tokenizeSegments(src, segmentsOf(node.Segments), func(int) int { return 0 }, func(z *html.Tokenizer, tt html.TokenType, raw []byte, _ int) {
				if (tt == html.StartTagToken || tt == html.SelfClosingTagToken) && strings.EqualFold(rawTagName(raw), "img") {
					b = append(b, ' ')
					b = append(b, imgAlt(z)...)
				}
			})
		}
		return true
	})
	return string(b)
}

// textValue returns the text of a Markdown text node with entities decoded.
// goldmark keeps &nbsp; as literal source text until it renders, so without
// decoding a cell holding only &nbsp; would look non-empty. Text in a code
// span renders literally, so `&nbsp;` there stays six visible characters.
func textValue(n *ast.Text, src []byte) string {
	v := string(n.Segment.Value(src))
	if _, ok := n.Parent().(*ast.CodeSpan); ok {
		return v
	}
	return html.UnescapeString(v)
}

// imgAlt returns the alt attribute of the img start tag z is on, which a
// screen reader announces as the image's text. It must run after rawTagName
// has read the raw token, because the tokenizer lowercases attribute names
// in place.
func imgAlt(z *html.Tokenizer) string {
	for {
		key, val, more := z.TagAttr()
		if string(key) == "alt" {
			return string(val)
		}
		if !more {
			return ""
		}
	}
}

// preorder visits n and its descendants in document order. visit returns
// false to skip a node's children. Unlike ast.Walk it has no error path,
// because none of these visitors can fail.
func preorder(n ast.Node, visit func(ast.Node) bool) {
	if !visit(n) {
		return
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		preorder(c, visit)
	}
}

// scanHTML tracks <table> and <th> through one raw-HTML node. State persists
// across nodes, so a table split over several HTML blocks is checked as one.
func (t *tableChecker) scanHTML(segs []text.Segment) {
	tokenizeSegments(t.c.src, segs, t.c.lineAt, func(z *html.Tokenizer, tt html.TokenType, raw []byte, line int) {
		switch tt {
		case html.TextToken:
			// Text decodes entities, so &nbsp; arrives as U+00A0.
			t.addText(string(z.Text()))
		case html.StartTagToken, html.SelfClosingTagToken:
			switch strings.ToLower(rawTagName(raw)) {
			case "img":
				t.addText(imgAlt(z))
			case "table":
				t.inTH = false
				t.tables = append(t.tables, nil)
			case "th":
				if len(t.tables) > 0 {
					top := len(t.tables) - 1
					t.tables[top] = append(t.tables[top], headerCell{line: line})
					t.inTH = true
				}
			case "td", "tr":
				t.inTH = false
			}
		case html.EndTagToken:
			switch strings.ToLower(rawTagName(raw)) {
			case "table":
				t.closeTable()
			case "th", "tr", "thead", "tbody", "tfoot":
				t.inTH = false
			}
		}
	})
}

// addText appends s to the open <th>, if any.
func (t *tableChecker) addText(s string) {
	if !t.inTH || len(t.tables) == 0 {
		return
	}
	cells := t.tables[len(t.tables)-1]
	last := &cells[len(cells)-1]
	last.text = collapse(last.text + " " + s)
}

// closeTable pops the innermost HTML table and reports its empty headers.
func (t *tableChecker) closeTable() {
	t.inTH = false
	top := len(t.tables) - 1
	cells := t.tables[top]
	t.tables = t.tables[:top]
	t.report(cells)
}

// report adds a finding for each empty cell, naming the table by its first
// non-empty header so the author can find it on a page with many tables.
func (t *tableChecker) report(cells []headerCell) {
	first := "(none)"
	for _, c := range cells {
		if c.text != "" {
			first = fmt.Sprintf("%q", c.text)
			break
		}
	}
	for i, c := range cells {
		if c.text != "" {
			continue
		}
		t.findings = append(t.findings, finding{
			line: c.line,
			kind: kindEmptyTableHeader,
			tag:  "th",
			msg:  fmt.Sprintf("header cell %d has no text (table's first header %s)", i+1, first),
		})
	}
}

// collapse trims s and folds internal whitespace into single spaces.
// strings.Fields splits on unicode.IsSpace, which includes the U+00A0 that
// &nbsp; decodes to, so a cell holding only &nbsp; collapses to "".
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
