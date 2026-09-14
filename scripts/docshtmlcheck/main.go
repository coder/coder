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
//   - Structurally invalid or unregistered HTML: end tags for void elements
//     (</br>); tag names outside the standard HTML5 element set; capitalized
//     tags such as <Image> or <Table>, which the docs renderer reads as
//     component references and drops when unregistered (only <children> is
//     registered); and unclosed container tags (a <div class="tabs"> that is
//     never closed and leaks its wrapper over the rest of the page).
//
// Detection is Markdown-aware: the file is parsed with goldmark and only raw
// HTML nodes are inspected, so angle brackets inside fenced code blocks, inline
// code spans, HTML comments, and <https://...> or <user@host> autolinks are
// ignored.
//
// Known limitations (accepted; no docs page hits either today): a placeholder
// whose name is itself a real element (<input>, <time>) is indistinguishable
// from intended markup and passes; and the interior of a raw-text element
// (<script>, <style>) is one opaque text token to the tokenizer, so a tag
// nested inside is not scanned, though an unclosed <script>/<style> is still
// caught.
//
// Usage:
//
//	docshtmlcheck [path ...]
//
// With no arguments it scans docs/. Arguments may be files or directories.
package main

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
// component (a child-page card grid with no HTML equivalent). Lookups are
// lowercase; a tag whose raw name is capitalized is treated as a component
// reference and reported by scanNode before this set is consulted, so only
// lowercase element names belong here. Any name outside the set is treated as
// a swallowed placeholder or an unregistered component and is reported. Inline
// SVG and MathML are intentionally out of scope (no docs page uses them); add
// the element here if that changes.
//
// Maintenance: this set is hand-maintained to mirror the tags the docs
// renderer actually accepts. It is not generated from the renderer, so a
// renderer change that adds or removes an accepted tag is not reflected here
// automatically and the two can drift until this map is updated by hand. Keep
// them in sync whenever the renderer's accepted set changes.
//
// Known gap: a placeholder whose name is itself a real element (<input>,
// <output>, <time>) is indistinguishable from intended markup and passes.
// Such placeholders almost always live in fenced code blocks, which are
// ignored, so the gap is narrow in practice.
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
	"ruby": true, "s": true, "samp": true, "script": true, "search": true,
	"section": true, "select": true, "slot": true, "small": true,
	"source": true, "span": true, "strong": true, "style": true, "sub": true,
	"summary": true, "sup": true, "table": true, "tbody": true, "td": true,
	"template": true, "textarea": true, "tfoot": true, "th": true,
	"thead": true, "time": true, "title": true, "tr": true, "track": true,
	"u": true, "ul": true, "var": true, "video": true, "wbr": true,

	// Intentional docs renderer component. Also listed in rendererComponents,
	// which drives the self-closing balance rule in scanNode.
	"children": true,
}

// rendererComponents are the non-HTML tags the docs renderer accepts as MDX
// components rather than standard HTML elements; they also appear in
// allowedElements. MDX honors the self-closing form on a component, so
// <children /> is complete and is not balance-tracked. A self-closing HTML
// container such as <div/> is different: the HTML5 parser ignores the flag and
// leaves it open, so it stays tracked and is still caught as an unclosed
// wrapper.
var rendererComponents = map[string]bool{
	"children": true,
}

// optionalEndTags are elements whose end tag is optional in the HTML5 parsing
// algorithm (a following sibling or the parent's end implicitly closes them).
// Requiring them to be explicitly balanced would produce false positives on
// valid HTML, so they are excluded from the unclosed/mismatch balance check.
// They are still subject to the void-end-tag and unknown-element checks.
var optionalEndTags = map[string]bool{
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
// The escape hatch is self-clearing: filterAllowed emits a stale-allowlist-entry
// finding (failing the build) if an allowlisted tag no longer appears in its
// file, so a dead entry cannot silently mask a future regression of the same
// tag on that page. The guard is per-tag on a scanned file; an entry whose
// file is deleted outright is never rescanned and lingers as harmless dead
// config (a missing file yields no findings, so nothing hides behind it).
//
// Temporary: docs/reference/cli/agent-firewall.md renders <host> and <glob>
// from the --session-id-inject-target help text, which is defined in the
// external github.com/coder/boundary CLI, not in this repo. The stale-entry
// guard removes the need to track removal by hand: once the upstream fix and
// dependency bump land and the generated page no longer contains the bare
// placeholders, the build fails until this entry is deleted.
var allowedUnknownTags = map[string]map[string]bool{
	"docs/reference/cli/agent-firewall.md": {"host": true, "glob": true},
}

// docshtmlcheckSource is where allowlist-maintenance findings are reported: the
// fix for a stale entry lives in this file's allowedUnknownTags, not in the
// scanned doc, so pointing at the doc would send the reader to the wrong file.
const docshtmlcheckSource = "scripts/docshtmlcheck/main.go"

type findingKind string

const (
	kindUnknownElement findingKind = "unknown-element"
	kindVoidEndTag     findingKind = "void-end-tag"
	kindUnclosedTag    findingKind = "unclosed-tag"
	kindStrayEndTag    findingKind = "stray-end-tag"
	kindStaleAllowlist findingKind = "stale-allowlist-entry"
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
	os.Exit(run(roots, os.Stdout, os.Stderr))
}

// run scans roots, writes per-finding lines to stdout and the summary and
// errors to stderr, and returns the process exit code (0 clean, 1 findings,
// 2 I/O error). It is separated from main so tests can drive it with buffers
// and assert the reported locations, the generator-source note routing, and
// the summary footers.
func run(roots []string, stdout, stderr io.Writer) int {
	files, err := collectMarkdown(roots)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "docshtmlcheck: %v\n", err)
		return 2
	}

	var htmlIssues, staleIssues int
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "docshtmlcheck: %v\n", err)
			return 2
		}
		h, s := reportFindings(path, filterAllowed(path, checkSource(src)), stdout)
		htmlIssues += h
		staleIssues += s
	}

	if htmlIssues > 0 {
		_, _ = fmt.Fprintf(stderr, "\ndocshtmlcheck: found %d invalid inline HTML issue(s).\n"+
			"Wrap angle-bracket placeholders in backticks so they render as inline code\n"+
			"(see docs/about/contributing/documentation.md), fix void-element end tags\n"+
			"like </br>, use registered components for custom tags, and close container tags.\n", htmlIssues)
	}
	if staleIssues > 0 {
		_, _ = fmt.Fprintf(stderr, "\ndocshtmlcheck: found %d stale allowlist entry(ies); "+
			"remove them from allowedUnknownTags in %s.\n", staleIssues, docshtmlcheckSource)
	}
	if htmlIssues+staleIssues > 0 {
		return 1
	}
	return 0
}

// reportFindings writes one file's findings to out and returns the count of
// invalid-HTML findings and stale-allowlist findings. A stale-allowlist finding
// is reported against the linter source (its fix lives in allowedUnknownTags,
// not the scanned doc) and with no line. When the file has a real content
// finding, is generated, and is not allowlisted, a trailing note points the
// author at the generator source instead of the throwaway output.
//
// The allowlist skip is file-scoped: a genuinely new invalid tag on an
// allowlisted generated page (for example a future boundary bump emitting
// <Foo> in agent-firewall.md) is still reported, but without the
// generator-source hint. That is acceptable for the single external page.
func reportFindings(path string, findings []finding, out io.Writer) (htmlIssues, staleIssues int) {
	realFinding := false
	for _, f := range findings {
		loc := path
		if f.kind == kindStaleAllowlist {
			// The fix is in this tool's allowlist, not the scanned doc, so
			// report the linter source rather than a misleading docs:1.
			loc = docshtmlcheckSource
		}
		if f.line > 0 {
			_, _ = fmt.Fprintf(out, "%s:%d: %s: %s\n", loc, f.line, f.kind, f.msg)
		} else {
			_, _ = fmt.Fprintf(out, "%s: %s: %s\n", loc, f.kind, f.msg)
		}
		if f.kind == kindStaleAllowlist {
			staleIssues++
		} else {
			htmlIssues++
			realFinding = true
		}
	}
	if realFinding && isGeneratedDoc(path) && allowedUnknownTags[canonicalPath(path)] == nil {
		_, _ = fmt.Fprintf(out, "%s: note: this page is generated by `make gen`; fix the source "+
			"(codersdk/*.go doc comments, CLI --help text, or swagger annotations) and "+
			"regenerate; edits to this file will not persist\n", path)
	}
	return htmlIssues, staleIssues
}

// collectMarkdown expands the given roots (files or directories) into a sorted
// list of unique .md files in canonical (repo-relative, slash) form.
func collectMarkdown(roots []string) ([]string, error) {
	seen := map[string]bool{}
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if strings.HasSuffix(root, ".md") {
				seen[canonicalPath(root)] = true
			}
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".md") {
				seen[canonicalPath(path)] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return slices.Sorted(maps.Keys(seen)), nil
}

// canonicalPath normalizes a path to a clean, slash-separated form, made
// relative to the working directory when absolute. Running the linter from the
// repo root (as CI and `make lint/docs-html` do) yields repo-relative keys such
// as docs/reference/cli/agent-firewall.md regardless of whether the caller
// passed a relative, ./-prefixed, or absolute path, so allowlist lookups and
// reported locations stay consistent.
func canonicalPath(p string) string {
	p = filepath.Clean(p)
	if filepath.IsAbs(p) {
		if wd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(wd, p); err == nil {
				p = rel
			}
		}
	}
	return filepath.ToSlash(p)
}

// isGeneratedDoc reports whether a docs path is produced by `make gen` rather
// than hand-written, so findings can route the author to the generator source.
func isGeneratedDoc(path string) bool {
	return strings.HasPrefix(canonicalPath(path), "docs/reference/")
}

// filterAllowed drops unknown-element findings suppressed by allowedUnknownTags
// for the file. It also reports any allowlist entry that suppressed nothing, so
// a stale escape hatch fails the build instead of silently masking a future
// regression of the same tag on that page.
func filterAllowed(path string, findings []finding) []finding {
	allowed := allowedUnknownTags[canonicalPath(path)]
	if allowed == nil {
		return findings
	}
	out := make([]finding, 0, len(findings))
	used := make(map[string]bool, len(allowed))
	for _, f := range findings {
		if f.kind == kindUnknownElement && allowed[f.tag] {
			used[f.tag] = true
			continue
		}
		out = append(out, f)
	}
	stale := make([]string, 0, len(allowed))
	for tag := range allowed {
		if !used[tag] {
			stale = append(stale, tag)
		}
	}
	slices.Sort(stale)
	for _, tag := range stale {
		// No line: the fix is in allowedUnknownTags, not at any line of the
		// scanned doc. main reports these against the linter source.
		out = append(out, finding{
			kind: kindStaleAllowlist,
			tag:  tag,
			msg: fmt.Sprintf("allowlist entry <%s> for %s no longer suppresses anything; "+
				"remove it from allowedUnknownTags", tag, canonicalPath(path)),
		})
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
			c.scanNode(segmentsOf(node.Segments))
		case *ast.HTMLBlock:
			segs := segmentsOf(node.Lines())
			if node.HasClosure() {
				segs = append(segs, node.ClosureLine)
			}
			c.scanNode(segs)
		}
		return ast.WalkContinue, nil
	})

	// Anything left open at end of file is unclosed.
	for _, open := range c.stack {
		c.findings = append(c.findings, unclosedFinding(open))
	}

	slices.SortStableFunc(c.findings, func(a, b finding) int {
		return cmp.Compare(a.line, b.line)
	})
	return c.findings
}

// segmentsOf materializes a *text.Segments into a slice.
func segmentsOf(s *text.Segments) []text.Segment {
	out := make([]text.Segment, 0, s.Len())
	for i := range s.Len() {
		out = append(out, s.At(i))
	}
	return out
}

// rawTagName extracts the original-case tag name from a raw token. The
// tokenizer's TagName lowercases, which hides whether a tag was capitalized, so
// scanNode reads the name straight from the raw bytes: skip the leading < and
// the / of an end tag, then take everything up to the first whitespace, / or >.
func rawTagName(raw []byte) string {
	i := 0
	for i < len(raw) && (raw[i] == '<' || raw[i] == '/') {
		i++
	}
	start := i
	for i < len(raw) {
		switch raw[i] {
		case ' ', '\t', '\n', '\r', '\f', '/', '>':
			return string(raw[start:i])
		}
		i++
	}
	return string(raw[start:i])
}

type openTag struct {
	tag  string
	line int
}

// unclosedFinding builds the finding for a container tag left open. It is the
// single source for the unclosed-tag message, shared by the end-of-file drain
// in checkSource and the dangling-inner-tag loop in matchEndTag.
func unclosedFinding(o openTag) finding {
	return finding{
		line: o.line,
		kind: kindUnclosedTag,
		tag:  o.tag,
		msg:  fmt.Sprintf("unclosed <%s> tag", o.tag),
	}
}

type checker struct {
	src        []byte
	lineStarts []int
	stack      []openTag
	findings   []finding
}

// scanNode tokenizes an entire raw-HTML node at once. It concatenates the
// node's source segments into a single buffer, so a tag whose text wraps
// across lines is tokenized whole instead of being torn in half. It maps each
// token back to its source line. The balance stack persists across nodes, so a
// container opened in one block and closed in another still balances.
func (c *checker) scanNode(segs []text.Segment) {
	if len(segs) == 0 {
		return
	}

	// Concatenate the segment text into one buffer, recording where each
	// segment lands so a buffer offset can be translated back to a source byte
	// offset. Segments are individually contiguous in the source but may be
	// separated by gaps (such as the line breaks a block spans).
	type span struct {
		bufStart, srcStart, length int
	}
	var buf []byte
	spans := make([]span, 0, len(segs))
	for _, seg := range segs {
		v := seg.Value(c.src)
		spans = append(spans, span{bufStart: len(buf), srcStart: seg.Start, length: len(v)})
		buf = append(buf, v...)
	}
	srcOffset := func(bufPos int) int {
		for i := len(spans) - 1; i >= 0; i-- {
			if bufPos >= spans[i].bufStart {
				return spans[i].srcStart + min(bufPos-spans[i].bufStart, spans[i].length)
			}
		}
		return spans[0].srcStart
	}

	z := html.NewTokenizer(bytes.NewReader(buf))
	pos := 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return
		}
		raw := z.Raw()
		line := c.lineAt(srcOffset(pos))
		pos += len(raw)

		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			rawName := rawTagName(raw)
			name := strings.ToLower(rawName)
			if autolinkShaped(name) {
				// e.g. <https://coder.com> or <user@coder.com>: valid Markdown
				// autolink syntax, never a real HTML element. goldmark only
				// classifies these as autolinks in inline context, not inside a
				// raw HTML block, so skip them explicitly to avoid false
				// positives.
				continue
			}
			if rawName != name {
				// A capitalized tag is an MDX/JSX component reference, not an
				// HTML element (element names are case-insensitive but
				// conventionally lowercase; the docs renderer treats a
				// capitalized tag as a component). Report it regardless of
				// whether the lowercased name collides with a real element, so
				// <Table>, <Section>, and <Image> are all caught.
				c.findings = append(c.findings, finding{
					line: line,
					kind: kindUnknownElement,
					tag:  name,
					msg: fmt.Sprintf("<%s> is a capitalized tag; the docs renderer reads it as a "+
						"component reference and drops it unless registered. Use a lowercase HTML "+
						"element or a registered component", rawName),
				})
				continue
			}
			if !allowedElements[name] {
				c.findings = append(c.findings, finding{
					line: line,
					kind: kindUnknownElement,
					tag:  name,
					msg: fmt.Sprintf("<%s> is not a recognized HTML element or component; wrap a "+
						"placeholder in backticks, add a standard HTML element to allowedElements, "+
						"or use a registered component", name),
				})
				continue
			}
			// Track balance for container elements that need an explicit end
			// tag. A self-closing flag on a non-void HTML element (<div/>) is
			// ignored by the HTML5 parser and the docs renderer, so it opens a
			// container that leaks exactly like <div>; keep tracking it. A
			// self-closing renderer component (<children/>) is the exception:
			// MDX honors the self-closing form, so it is complete and must not
			// be tracked. Void and optional-end-tag elements are never pushed.
			if !voidElements[name] && !optionalEndTags[name] {
				if tt != html.SelfClosingTagToken || !rendererComponents[name] {
					c.stack = append(c.stack, openTag{tag: name, line: line})
				}
			}
		case html.EndTagToken:
			rawName := rawTagName(raw)
			name := strings.ToLower(rawName)
			if autolinkShaped(name) {
				continue
			}
			if rawName != name {
				// Closing tag of a capitalized component; its opening tag was
				// already reported, and component tags are not balance-tracked.
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
			if !allowedElements[name] || optionalEndTags[name] {
				// Unknown end tags are reported via their start tag; optional
				// end tags are not balance-tracked.
				continue
			}
			c.matchEndTag(name, line)
		}
	}
}

// matchEndTag resolves an end tag against the balance stack: it pops to the
// matching open tag, emits kindUnclosedTag for any tags left dangling above the
// match, and emits kindStrayEndTag when nothing matches.
func (c *checker) matchEndTag(name string, line int) {
	for i := len(c.stack) - 1; i >= 0; i-- {
		if c.stack[i].tag == name {
			// Tags above the match were left unclosed inside this element.
			for j := len(c.stack) - 1; j > i; j-- {
				c.findings = append(c.findings, unclosedFinding(c.stack[j]))
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

// schemeRe matches a URI scheme ending in a colon (e.g. "https:", "mailto:").
// A Markdown autolink inside a raw-HTML block tokenizes with such a name.
var schemeRe = regexp.MustCompile(`^[a-z][a-z0-9+.-]*:$`)

// autolinkShaped reports whether a tokenized tag name is a real Markdown
// autolink rather than an HTML element or a placeholder. goldmark only
// classifies autolinks as such in inline context; inside a raw-HTML block one
// tokenizes as a tag whose name is either a URI scheme ending in a colon
// (<https://coder.com> tokenizes as "https:") or a mail-shaped local@domain
// with a real, dotted domain (<user@coder.com>). A bare colon or at sign is
// not enough, so placeholders the linter targets (<region:id>, <user@host>)
// stay checked.
func autolinkShaped(name string) bool {
	if schemeRe.MatchString(name) {
		return true
	}
	at := strings.IndexByte(name, '@')
	if at <= 0 || at >= len(name)-1 {
		return false
	}
	domain := name[at+1:]
	return !strings.Contains(domain, "@") && strings.Contains(domain, ".")
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
