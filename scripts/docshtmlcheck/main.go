// Command docshtmlcheck fails when Markdown files under docs/ contain invalid
// inline HTML that the documentation site's HTML renderer silently drops or
// mangles.
//
// It exists to prevent regressions of two classes of bug that were fixed by a
// manual audit of the docs:
//
//   - Swallowed angle-bracket placeholders. An unwrapped placeholder such as
//     <region> or <server>__ is parsed as an unknown HTML tag and stripped
//     from the rendered page, so readers see broken text. Placeholders must be
//     wrapped in backticks (see docs/about/contributing/documentation.md).
//     This also covers CLI --help strings and Swagger annotations, whose text
//     is generated into docs/reference/**.
//   - Structurally invalid HTML: end tags for void elements (</br>),
//     capitalized component tags (<Image> instead of <img>), and unclosed
//     container tags (a <div class="tabs"> that is never closed and leaks its
//     wrapper over the rest of the page).
//
// Detection is Markdown-aware: the file is parsed with goldmark and only raw
// HTML nodes are inspected, so angle brackets inside fenced code blocks, inline
// code spans, HTML comments, and <https://...> autolinks are ignored.
//
// Usage:
//
//	docshtmlcheck [path ...]
//
// With no arguments it scans docs/. Arguments may be files or directories.
package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"golang.org/x/net/html"
)

// voidElements are HTML elements that never have an end tag. An end tag for any
// of these (e.g. </br>) is invalid.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// allowedElements is the set of tag names permitted in docs Markdown: the
// standard HTML5 element set plus "children", the one intentional renderer
// component (a child-page card grid with no HTML equivalent). Any tag outside
// this set is treated as a swallowed placeholder or an unregistered component
// and is reported.
var allowedElements = map[string]bool{
	// Standard HTML5 elements.
	"a": true, "abbr": true, "address": true, "area": true, "article": true,
	"aside": true, "audio": true, "b": true, "base": true, "bdi": true,
	"bdo": true, "blockquote": true, "body": true, "br": true, "button": true,
	"canvas": true, "caption": true, "cite": true, "code": true, "col": true,
	"colgroup": true, "data": true, "datalist": true, "dd": true, "del": true,
	"details": true, "dfn": true, "dialog": true, "div": true, "dl": true,
	"dt": true, "em": true, "embed": true, "fieldset": true, "figcaption": true,
	"figure": true, "footer": true, "form": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true, "head": true, "header": true,
	"hgroup": true, "hr": true, "html": true, "i": true, "iframe": true,
	"img": true, "input": true, "ins": true, "kbd": true, "label": true,
	"legend": true, "li": true, "link": true, "main": true, "map": true,
	"mark": true, "menu": true, "meta": true, "meter": true, "nav": true,
	"noscript": true, "object": true, "ol": true, "optgroup": true,
	"option": true, "output": true, "p": true, "param": true, "picture": true,
	"pre": true, "progress": true, "q": true, "rp": true, "rt": true,
	"ruby": true, "s": true, "samp": true, "script": true, "section": true,
	"select": true, "slot": true, "small": true, "source": true, "span": true,
	"strong": true, "style": true, "sub": true, "summary": true, "sup": true,
	"table": true, "tbody": true, "td": true, "template": true, "textarea": true,
	"tfoot": true, "th": true, "thead": true, "time": true, "title": true,
	"tr": true, "track": true, "u": true, "ul": true, "var": true, "video": true,
	"wbr": true,

	// Intentional docs renderer component.
	"children": true,
}

// optionalEndTag are elements whose end tag is optional in the HTML5 parsing
// algorithm (a following sibling or the parent's end implicitly closes them).
// Requiring them to be explicitly balanced would produce false positives on
// valid HTML, so they are excluded from the unclosed/mismatch balance check.
// They are still subject to the void-end-tag and unknown-element checks.
var optionalEndTag = map[string]bool{
	"li": true, "dd": true, "dt": true, "p": true, "option": true,
	"optgroup": true, "td": true, "th": true, "tr": true, "thead": true,
	"tbody": true, "tfoot": true, "caption": true, "colgroup": true,
	"rt": true, "rp": true,
}

// allowedUnknownTags suppresses specific unknown-element findings on specific
// files. This is a deliberately narrow escape hatch for placeholders whose
// source is outside this repository and therefore cannot be fixed by a source
// edit here.
//
// Temporary: docs/reference/cli/agent-firewall.md renders <host> and <glob>
// from the --session-id-inject-target help text, which is defined in the
// external github.com/coder/boundary CLI, not in this repo. Remove this entry
// once the upstream fix and the dependency bump land and the generated page no
// longer contains the bare placeholders.
var allowedUnknownTags = map[string]map[string]bool{
	"docs/reference/cli/agent-firewall.md": {"host": true, "glob": true},
}

type findingKind string

const (
	kindUnknownElement findingKind = "unknown-element"
	kindVoidEndTag     findingKind = "void-end-tag"
	kindUnclosed       findingKind = "unclosed-tag"
	kindStrayEndTag    findingKind = "stray-end-tag"
)

type finding struct {
	line int
	kind findingKind
	tag  string
	msg  string
}

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"docs"}
	}

	files, err := collectMarkdown(roots)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "docshtmlcheck: %v\n", err)
		os.Exit(2)
	}

	_, _ = fmt.Println("--- check for invalid inline HTML in docs")

	total := 0
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "docshtmlcheck: %v\n", err)
			os.Exit(2)
		}
		findings := filterAllowed(path, checkSource(src))
		for _, f := range findings {
			_, _ = fmt.Printf("%s:%d: %s: %s\n", path, f.line, f.kind, f.msg)
			total++
		}
	}

	if total > 0 {
		_, _ = fmt.Fprintf(os.Stderr, "\ndocshtmlcheck: found %d invalid inline HTML issue(s).\n"+
			"Wrap angle-bracket placeholders in backticks so they render as inline code\n"+
			"(see docs/about/contributing/documentation.md), fix void-element end tags\n"+
			"like </br>, lowercase component tags, and close container tags.\n", total)
		os.Exit(1)
	}
}

// collectMarkdown expands the given roots (files or directories) into a sorted
// list of .md files.
func collectMarkdown(roots []string) ([]string, error) {
	seen := map[string]bool{}
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if strings.HasSuffix(root, ".md") {
				seen[filepath.ToSlash(root)] = true
			}
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".md") {
				seen[filepath.ToSlash(path)] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	return files, nil
}

// filterAllowed drops findings suppressed by allowedUnknownTags for the file.
func filterAllowed(path string, findings []finding) []finding {
	allowed := allowedUnknownTags[filepath.ToSlash(path)]
	if allowed == nil {
		return findings
	}
	out := findings[:0]
	for _, f := range findings {
		if f.kind == kindUnknownElement && allowed[f.tag] {
			continue
		}
		out = append(out, f)
	}
	return out
}

// checkSource parses Markdown and returns findings for invalid inline HTML. It
// inspects only raw HTML nodes, so angle brackets inside code spans, fenced
// code blocks, HTML comments, and autolinks are ignored.
func checkSource(src []byte) []finding {
	doc := goldmark.New().Parser().Parse(text.NewReader(src))

	c := &checker{src: src, lineStarts: lineStarts(src)}

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.RawHTML:
			for i := 0; i < node.Segments.Len(); i++ {
				seg := node.Segments.At(i)
				c.scan(seg.Value(src), seg.Start)
			}
		case *ast.HTMLBlock:
			lines := node.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				c.scan(seg.Value(src), seg.Start)
			}
			if node.HasClosure() {
				seg := node.ClosureLine
				c.scan(seg.Value(src), seg.Start)
			}
		}
		return ast.WalkContinue, nil
	})

	// Anything left open at end of file is unclosed.
	for _, open := range c.stack {
		c.findings = append(c.findings, finding{
			line: open.line,
			kind: kindUnclosed,
			tag:  open.tag,
			msg:  fmt.Sprintf("unclosed <%s> tag", open.tag),
		})
	}

	sort.SliceStable(c.findings, func(i, j int) bool {
		return c.findings[i].line < c.findings[j].line
	})
	return c.findings
}

type openTag struct {
	tag  string
	line int
}

type checker struct {
	src        []byte
	lineStarts []int
	stack      []openTag
	findings   []finding
}

// scan tokenizes a single raw-HTML fragment and updates the checker's balance
// stack and findings. startOffset is the byte offset of the fragment in the
// source, used to report line numbers.
func (c *checker) scan(fragment []byte, startOffset int) {
	line := c.lineAt(startOffset)
	z := html.NewTokenizer(bytes.NewReader(fragment))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return
		}
		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			nameBytes, _ := z.TagName()
			name := strings.ToLower(string(nameBytes))
			if autolinkShaped(name) {
				// e.g. <https://...> or <user@host>: valid Markdown autolink
				// syntax, never a real HTML element. goldmark only classifies
				// these as autolinks in inline context, not inside a raw HTML
				// block, so skip them explicitly to avoid false positives.
				continue
			}
			if !allowedElements[name] {
				c.findings = append(c.findings, finding{
					line: line,
					kind: kindUnknownElement,
					tag:  name,
					msg: fmt.Sprintf("<%s> is not valid inline HTML; if it is a placeholder, "+
						"wrap it in backticks, otherwise use a registered component", name),
				})
				continue
			}
			// Only track balance for container elements that require an
			// explicit end tag. Void elements and optional-end-tag elements
			// are never pushed.
			if tt == html.StartTagToken && !voidElements[name] && !optionalEndTag[name] {
				c.stack = append(c.stack, openTag{tag: name, line: line})
			}
		case html.EndTagToken:
			nameBytes, _ := z.TagName()
			name := strings.ToLower(string(nameBytes))
			if autolinkShaped(name) {
				continue
			}
			if voidElements[name] {
				c.findings = append(c.findings, finding{
					line: line,
					kind: kindVoidEndTag,
					tag:  name,
					msg:  fmt.Sprintf("</%s> is invalid: <%s> is a void element with no end tag", name, name),
				})
				continue
			}
			if !allowedElements[name] || optionalEndTag[name] {
				// Unknown end tags are reported via their start tag; optional
				// end tags are not balance-tracked.
				continue
			}
			c.pop(name, line)
		}
	}
}

// pop matches an end tag against the balance stack.
func (c *checker) pop(name string, line int) {
	for i := len(c.stack) - 1; i >= 0; i-- {
		if c.stack[i].tag == name {
			// Anything above the match was left unclosed inside this element;
			// report them, then pop through the match.
			for j := len(c.stack) - 1; j > i; j-- {
				c.findings = append(c.findings, finding{
					line: c.stack[j].line,
					kind: kindUnclosed,
					tag:  c.stack[j].tag,
					msg:  fmt.Sprintf("unclosed <%s> tag", c.stack[j].tag),
				})
			}
			c.stack = c.stack[:i]
			return
		}
	}
	c.findings = append(c.findings, finding{
		line: line,
		kind: kindStrayEndTag,
		tag:  name,
		msg:  fmt.Sprintf("</%s> has no matching opening tag", name),
	})
}

// autolinkShaped reports whether a tokenized tag name looks like a Markdown
// autolink rather than an HTML element. Autolinks such as <https://coder.com>
// or <user@coder.com> tokenize as tags whose "name" contains a scheme colon or
// an at sign; real HTML element names contain neither. Placeholders the linter
// targets (<region>, <host>, <organization-name>) never match this.
func autolinkShaped(name string) bool {
	return strings.ContainsAny(name, ":@")
}

// lineStarts returns the byte offset of the start of each line.
func lineStarts(src []byte) []int {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// lineAt returns the 1-based line number for a byte offset.
func (c *checker) lineAt(offset int) int {
	// Largest index i such that lineStarts[i] <= offset.
	i := sort.Search(len(c.lineStarts), func(i int) bool {
		return c.lineStarts[i] > offset
	})
	if i < 1 {
		return 1
	}
	return i
}
