package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCheckSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
		// want maps each expected finding kind to the tag it concerns. Order
		// independent; the test asserts the multiset of (kind, tag) pairs.
		want []finding
	}{
		{
			name: "swallowed placeholder in prose",
			src:  "Use `--region` set to <region> for the endpoint.\n",
			want: []finding{{kind: kindUnknownElement, tag: "region"}},
		},
		{
			name: "placeholder inside inline code is ignored",
			src:  "Constructs `https://bedrock-runtime.<region>.amazonaws.com`.\n",
			want: nil,
		},
		{
			name: "placeholder inside fenced code block is ignored",
			src:  "```\nendpoint: https://host.<region>.example.com\n```\n",
			want: nil,
		},
		{
			name: "placeholder inside indented code block is ignored",
			src:  "    literal <region> here\n",
			want: nil,
		},
		{
			name: "double-underscore placeholder in prose",
			src:  "The tool name has the <server>__ prefix stripped.\n",
			want: []finding{{kind: kindUnknownElement, tag: "server"}},
		},
		{
			name: "void element end tag",
			src:  "First line.</br>\nSecond line.\n",
			want: []finding{{kind: kindVoidEndTag, tag: "br"}},
		},
		{
			name: "void element self and start tags are fine",
			src:  "A<br>B<br/>C <img src=\"x.png\"> D\n",
			want: nil,
		},
		{
			name: "capitalized component tag",
			src:  "<Image src=\"x.png\">\n",
			want: []finding{{kind: kindUnknownElement, tag: "image"}},
		},
		{
			name: "balanced kbd inline",
			src:  "Press <kbd>Ctrl</kbd> to continue.\n",
			want: nil,
		},
		{
			name: "balanced children component",
			src:  "<children></children>\n",
			want: nil,
		},
		{
			name: "unbalanced children component",
			src:  "<children>\n\nsome text\n",
			want: []finding{{kind: kindUnclosedTag, tag: "children"}},
		},
		{
			name: "unclosed div leaks wrapper",
			src:  "<div class=\"tabs\">\n\n## Heading\n\ncontent\n",
			want: []finding{{kind: kindUnclosedTag, tag: "div"}},
		},
		{
			name: "balanced div block",
			src:  "<div class=\"tabs\">\n\n## Heading\n\n</div>\n",
			want: nil,
		},
		{
			name: "stray end tag",
			src:  "text</div>\n",
			want: []finding{{kind: kindStrayEndTag, tag: "div"}},
		},
		{
			// Regression guard: a start tag whose attributes wrap across lines
			// must be tokenized whole, not torn in half. A per-line tokenizer
			// silently dropped this unclosed <div> (exit 0), the exact leaked
			// wrapper the linter exists to catch.
			name: "unclosed div with attributes wrapped across lines",
			src:  "<div\n  class=\"tabs\">\n\n## Heading\n\ncontent\n",
			want: []finding{{kind: kindUnclosedTag, tag: "div"}},
		},
		{
			name: "unknown element with attributes wrapped across lines",
			src:  "See <Image\n  src=\"x.png\"> here.\n",
			want: []finding{{kind: kindUnknownElement, tag: "image"}},
		},
		{
			// A valid inline tag wrapped across lines must NOT produce a
			// spurious stray-end-tag on the closing tag.
			name: "balanced kbd wrapped across lines",
			src:  "Press <kbd\n  class=\"key\">Ctrl</kbd> now.\n",
			want: nil,
		},
		{
			// Interleaved nesting: the inner tag is left dangling when the outer
			// tag closes, exercising matchEndTag's unclosed-reporting loop.
			name: "inner tag unclosed when outer closes",
			src:  "<div>\n\n<span>\n\n</div>\n",
			want: []finding{{kind: kindUnclosedTag, tag: "span"}},
		},
		{
			name: "autolink is not raw html",
			src:  "See <https://coder.com/docs> for details.\n",
			want: nil,
		},
		{
			name: "autolink inside a children raw-html block is ignored",
			src:  "<children>\n  This page is rendered on <https://coder.com/docs/tutorials>.\n</children>\n",
			want: nil,
		},
		{
			name: "email autolink is not raw html",
			src:  "Contact <support@coder.com> for help.\n",
			want: nil,
		},
		{
			name: "placeholder inside html comment is ignored",
			src:  "<!-- TODO: document <region> here -->\n",
			want: nil,
		},
		{
			name: "optional end tag li is not flagged as unclosed",
			src:  "<ul>\n<li>one\n<li>two\n</ul>\n",
			want: nil,
		},
		{
			name: "table cells with optional end tags are fine",
			src:  "<table><tr><td>a<td>b</tr></table>\n",
			want: nil,
		},
		{
			name: "clean prose with angle-bracket math is not html",
			src:  "If a < b and b > c then done.\n",
			want: nil,
		},
		{
			// A self-closing flag on a non-void HTML container is ignored by
			// the HTML5 parser, so <div class="tabs"/> leaks its wrapper
			// exactly like the open spelling and must still be caught.
			name: "self-closing div still leaks wrapper",
			src:  "<div class=\"tabs\"/>\n\n## Heading\n\ncontent\n",
			want: []finding{{kind: kindUnclosedTag, tag: "div"}},
		},
		{
			// A renderer component is not an HTML element: MDX honors the
			// self-closing form, so <children/> is complete and must not be
			// flagged as unclosed, even though <div/> above is.
			name: "self-closing children component is balanced",
			src:  "<children/>\n",
			want: nil,
		},
		{
			name: "self-closing children component with space is balanced",
			src:  "<children />\n",
			want: nil,
		},
		{
			// A capitalized tag whose lowercase name is a real element
			// (<Table>, <Section>) is a component reference, not that element,
			// and is reported rather than passing on the accidental lookup.
			name: "capitalized real-element name is a component",
			src:  "<Table>\n",
			want: []finding{{kind: kindUnknownElement, tag: "table"}},
		},
		{
			name: "another capitalized real-element name is a component",
			src:  "<Section>\n\ncontent\n",
			want: []finding{{kind: kindUnknownElement, tag: "section"}},
		},
		{
			// The capitalized closing tag must not add a spurious stray-end-tag;
			// the opening tag already produced the finding.
			name: "capitalized component with close tag reports once",
			src:  "<Table>\n\ncontent\n\n</Table>\n",
			want: []finding{{kind: kindUnknownElement, tag: "table"}},
		},
		{
			// A colon-shaped placeholder is not a real autolink, so it is
			// checked (and reported) inside a raw-HTML block.
			name: "colon-shaped placeholder in raw block is caught",
			src:  "<div>\n<region:id>\n</div>\n",
			want: []finding{{kind: kindUnknownElement, tag: "region:id"}},
		},
		{
			name: "at-shaped placeholder without a domain dot is caught",
			src:  "<div>\n<user@host>\n</div>\n",
			want: []finding{{kind: kindUnknownElement, tag: "user@host"}},
		},
		{
			// A real dotted email is a genuine autolink and stays ignored, even
			// inside a raw-HTML block.
			name: "email autolink inside raw block is ignored",
			src:  "<div>\n<ops@coder.com>\n</div>\n",
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := checkSource([]byte(tc.src))
			assertFindings(t, tc.want, got)
		})
	}
}

// TestFindingLine pins the reported line number so the line-tracking machinery
// (lineStarts, lineAt, offset mapping) cannot silently regress to a constant.
func TestFindingLine(t *testing.T) {
	t.Parallel()

	// 1: "intro", 2: blank, 3: the unclosed <div>.
	src := "intro\n\n<div class=\"tabs\">\n\n## Heading\n\ncontent\n"
	got := checkSource([]byte(src))
	if len(got) != 1 {
		t.Fatalf("want exactly 1 finding, got %d: %+v", len(got), got)
	}
	if got[0].kind != kindUnclosedTag || got[0].tag != "div" {
		t.Fatalf("want unclosed <div>, got %+v", got[0])
	}
	if got[0].line != 3 {
		t.Errorf("want unclosed <div> reported on line 3, got line %d", got[0].line)
	}
}

// TestFindingLineWrapped pins the source line of a finding that is not the
// first token in its raw-HTML block, so the buffer-offset -> source-line
// mapping (srcOffset/spans) is actually exercised. A mapping that collapsed
// every token to the block start would report line 1 here instead of line 2.
func TestFindingLineWrapped(t *testing.T) {
	t.Parallel()

	// 1: <section> opens, 2: <Foo> is the finding, 3: </section> closes.
	src := "<section>\n<Foo>\n</section>\n"
	got := checkSource([]byte(src))
	if len(got) != 1 {
		t.Fatalf("want exactly 1 finding, got %d: %+v", len(got), got)
	}
	if got[0].kind != kindUnknownElement || got[0].tag != "foo" {
		t.Fatalf("want unknown-element <Foo>, got %+v", got[0])
	}
	if got[0].line != 2 {
		t.Errorf("want <Foo> reported on line 2, got line %d", got[0].line)
	}
}

// TestIsGeneratedDoc covers the generated-doc routing branch, which no current
// docs page triggers (every placeholder was fixed at its source), so only a
// test exercises it.
func TestIsGeneratedDoc(t *testing.T) {
	t.Parallel()

	if !isGeneratedDoc("docs/reference/cli/server.md") {
		t.Error("want docs/reference/** treated as generated")
	}
	if isGeneratedDoc("docs/admin/security/audit-logs.md") {
		t.Error("want a hand-written docs path treated as not generated")
	}
}

func TestFilterAllowed(t *testing.T) {
	t.Parallel()

	// agent-firewall's <host>/<glob> come from an external dependency and are
	// suppressed on that specific path only.
	src := "Format: \"domain=<host> [path=<glob>]\".\n"
	raw := checkSource([]byte(src))
	if len(raw) != 2 {
		t.Fatalf("expected 2 raw findings, got %d: %+v", len(raw), raw)
	}

	filtered := filterAllowed("docs/reference/cli/agent-firewall.md", checkSource([]byte(src)))
	if len(filtered) != 0 {
		t.Fatalf("expected agent-firewall host/glob to be suppressed, got %+v", filtered)
	}

	// The same tokens are NOT suppressed on any other path.
	other := filterAllowed("docs/reference/cli/other.md", checkSource([]byte(src)))
	if len(other) != 2 {
		t.Fatalf("expected 2 findings on non-allowlisted path, got %d: %+v", len(other), other)
	}
}

// TestFilterAllowedStaleEntry verifies the self-clearing guard: when an
// allowlisted tag no longer appears in its file, the unused entry surfaces as a
// stale-allowlist-entry finding so the dead escape hatch fails the build.
func TestFilterAllowedStaleEntry(t *testing.T) {
	t.Parallel()

	// Only <host> remains; the <glob> allowlist entry now suppresses nothing.
	src := "Format: \"domain=<host>\".\n"
	got := filterAllowed("docs/reference/cli/agent-firewall.md", checkSource([]byte(src)))
	if len(got) != 1 {
		t.Fatalf("want exactly 1 stale finding, got %d: %+v", len(got), got)
	}
	if got[0].kind != kindStaleAllowlist || got[0].tag != "glob" {
		t.Fatalf("want stale-allowlist-entry for <glob>, got %+v", got[0])
	}
	// The fix lives in the linter's allowlist, not at any doc line, so the
	// finding carries no line (main reports it against the linter source).
	if got[0].line != 0 {
		t.Errorf("want stale finding to carry no doc line, got line %d", got[0].line)
	}
	if !strings.Contains(got[0].msg, "allowedUnknownTags") {
		t.Errorf("want stale message to name allowedUnknownTags, got %q", got[0].msg)
	}
}

func TestCollectMarkdown(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "b.md"))
	writeTestFile(t, filepath.Join(dir, "a.md"))
	writeTestFile(t, filepath.Join(dir, "sub", "c.md"))
	writeTestFile(t, filepath.Join(dir, "ignore.txt"))

	got, err := collectMarkdown([]string{dir})
	if err != nil {
		t.Fatalf("collectMarkdown: %v", err)
	}
	// Expected keys go through canonicalPath just like collectMarkdown's, so
	// the assertion holds regardless of the temp dir's absolute location.
	want := []string{
		canonicalPath(filepath.Join(dir, "a.md")),
		canonicalPath(filepath.Join(dir, "b.md")),
		canonicalPath(filepath.Join(dir, "sub", "c.md")),
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("want %v (sorted, deduped, .md only), got %v", want, got)
	}
}

// TestReportFindings covers the per-file reporting in run: the reported
// location (linter source vs. scanned doc), the generator-source note routing
// per file class, and the returned counts. No current docs page triggers these
// branches, so only a test exercises them.
func TestReportFindings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		path         string
		findings     []finding
		wantContains []string
		wantAbsent   []string
		wantHTML     int
		wantStale    int
	}{
		{
			name:         "generated page routes to the generator source",
			path:         "docs/reference/cli/server.md",
			findings:     []finding{{line: 3, kind: kindUnknownElement, tag: "region", msg: "swallowed"}},
			wantContains: []string{"docs/reference/cli/server.md:3: unknown-element: swallowed", "generated by `make gen`"},
			wantHTML:     1,
		},
		{
			// An allowlisted external page is generated, but its placeholders
			// come from an external CLI, so the generator-source note is skipped.
			name:         "allowlisted external page omits the generator note",
			path:         "docs/reference/cli/agent-firewall.md",
			findings:     []finding{{line: 5, kind: kindUnknownElement, tag: "foo", msg: "swallowed"}},
			wantContains: []string{"docs/reference/cli/agent-firewall.md:5: unknown-element: swallowed"},
			wantAbsent:   []string{"generated by `make gen`"},
			wantHTML:     1,
		},
		{
			name:       "hand-written page omits the generator note",
			path:       "docs/admin/security/audit-logs.md",
			findings:   []finding{{line: 2, kind: kindUnknownElement, tag: "foo", msg: "swallowed"}},
			wantAbsent: []string{"generated by `make gen`"},
			wantHTML:   1,
		},
		{
			// A stale-allowlist finding is reported against the linter source
			// with no line, counts as stale (not HTML), and never draws the note.
			name:         "stale entry reports against the linter source",
			path:         "docs/reference/cli/agent-firewall.md",
			findings:     []finding{{kind: kindStaleAllowlist, tag: "glob", msg: "remove it from allowedUnknownTags"}},
			wantContains: []string{docshtmlcheckSource + ": stale-allowlist-entry: remove it from allowedUnknownTags"},
			wantAbsent:   []string{"generated by `make gen`", "agent-firewall.md:"},
			wantStale:    1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out strings.Builder
			html, stale := reportFindings(tc.path, tc.findings, &out)
			got := out.String()
			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("output should not contain %q\ngot:\n%s", absent, got)
				}
			}
			if html != tc.wantHTML || stale != tc.wantStale {
				t.Errorf("want (html=%d stale=%d), got (html=%d stale=%d)", tc.wantHTML, tc.wantStale, html, stale)
			}
		})
	}
}

// TestRun drives run end to end through temp files and asserts the exit-code
// contract: 0 clean, 1 findings, 2 unreadable root.
func TestRun(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	clean := filepath.Join(dir, "clean.md")
	if err := os.WriteFile(clean, []byte("# ok\n\nPress <kbd>Ctrl</kbd>.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{clean}, io.Discard, io.Discard); code != 0 {
		t.Errorf("clean file: want exit 0, got %d", code)
	}

	bad := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(bad, []byte("A <region> placeholder.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	if code := run([]string{bad}, &stdout, &stderr); code != 1 {
		t.Errorf("file with a finding: want exit 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "unknown-element") {
		t.Errorf("want an unknown-element line, got:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "invalid inline HTML issue(s)") {
		t.Errorf("want the summary footer, got:\n%s", stderr.String())
	}

	if code := run([]string{filepath.Join(dir, "does-not-exist")}, io.Discard, io.Discard); code != 2 {
		t.Errorf("missing root: want exit 2, got %d", code)
	}
}

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# doc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertFindings(t *testing.T, want, got []finding) {
	t.Helper()
	type key struct {
		kind findingKind
		tag  string
	}
	counts := map[key]int{}
	for _, f := range got {
		counts[key{f.kind, f.tag}]++
	}
	wantCounts := map[key]int{}
	for _, f := range want {
		wantCounts[key{f.kind, f.tag}]++
	}
	for k, n := range wantCounts {
		if counts[k] != n {
			t.Errorf("want %d finding(s) of {%s %s}, got %d\nall findings: %+v", n, k.kind, k.tag, counts[k], got)
		}
	}
	for k, n := range counts {
		if wantCounts[k] == 0 {
			t.Errorf("unexpected %d finding(s) of {%s %s}\nall findings: %+v", n, k.kind, k.tag, got)
		}
	}
}
