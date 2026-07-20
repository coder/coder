package main

import (
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
			want: []finding{{kind: kindUnclosed, tag: "children"}},
		},
		{
			name: "unclosed div leaks wrapper",
			src:  "<div class=\"tabs\">\n\n## Heading\n\ncontent\n",
			want: []finding{{kind: kindUnclosed, tag: "div"}},
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
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := checkSource([]byte(tc.src))
			assertFindings(t, tc.want, got)
		})
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
