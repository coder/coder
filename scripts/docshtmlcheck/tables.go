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
// whitespace is trimmed, so <th>&nbsp;</th> and <th><br></th> count as empty.
// Tables inside code blocks and code spans are ignored.
func checkTables(src []byte) []finding {
	doc := goldmark.New(goldmark.WithExtensions(extension.Table)).Parser().Parse(text.NewReader(src))
	t := &tableChecker{c: checker{src: src, lineStarts: lineStarts(src)}}

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *east.TableHeader:
			t.pipeHeader(node)
			return ast.WalkSkipChildren, nil
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
			t.addText(string(node.Segment.Value(src)))
		case *ast.String:
			t.addText(string(node.Value))
		}
		return ast.WalkContinue, nil
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
	var b strings.Builder
	_ = ast.Walk(n, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Text:
			b.Write(node.Segment.Value(src))
		case *ast.String:
			b.Write(node.Value)
		case *ast.AutoLink:
			b.Write(node.Label(src))
		}
		return ast.WalkContinue, nil
	})
	return b.String()
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
