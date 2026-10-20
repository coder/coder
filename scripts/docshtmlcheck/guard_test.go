package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckSourceDisallowed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
		want []finding
	}{
		{
			name: "iframe embed",
			src:  "<iframe src=\"https://www.youtube.com/embed/x\"></iframe>\n",
			want: []finding{{kind: kindDisallowedElement, tag: "iframe"}},
		},
		{
			name: "object embed",
			src:  "<object data=\"diagram.svg\"></object>\n",
			want: []finding{{kind: kindDisallowedElement, tag: "object"}},
		},
		{
			name: "image map area",
			src:  "<map name=\"m\"><area href=\"#a\" coords=\"0,0,1,1\"></map>\n",
			want: []finding{{kind: kindDisallowedElement, tag: "area"}},
		},
		{
			name: "form and its controls",
			src:  "<form>\n<input type=\"text\">\n<select><option>a</option></select>\n<textarea></textarea>\n<button>Go</button>\n</form>\n",
			want: []finding{
				{kind: kindDisallowedElement, tag: "form"},
				{kind: kindDisallowedElement, tag: "input"},
				{kind: kindDisallowedElement, tag: "select"},
				{kind: kindDisallowedElement, tag: "textarea"},
				{kind: kindDisallowedElement, tag: "button"},
			},
		},
		{
			name: "meta refresh",
			src:  "<meta http-equiv=\"refresh\" content=\"0; url=/docs\">\n",
			want: []finding{{kind: kindDisallowedElement, tag: "meta"}},
		},
		{
			// The element is reported, and its end tag still balances, so a
			// closed <button> adds no stray-end-tag or unclosed-tag finding.
			name: "inline button reports once",
			src:  "Select <button>Save</button> to finish.\n",
			want: []finding{{kind: kindDisallowedElement, tag: "button"}},
		},
		{
			name: "aria attribute",
			src:  "<span aria-label=\"Close\">x</span>\n",
			want: []finding{{kind: kindDisallowedAttribute, tag: "span"}},
		},
		{
			name: "role attribute",
			src:  "<div role=\"button\">\n\ncontent\n\n</div>\n",
			want: []finding{{kind: kindDisallowedAttribute, tag: "div"}},
		},
		{
			name: "tabindex attribute",
			src:  "<a href=\"#x\" tabindex=\"1\">jump</a>\n",
			want: []finding{{kind: kindDisallowedAttribute, tag: "a"}},
		},
		{
			name: "lang attribute",
			src:  "<span lang=\"fr\">bonjour</span>\n",
			want: []finding{{kind: kindDisallowedAttribute, tag: "span"}},
		},
		{
			name: "attribute names are case-insensitive",
			src:  "<span ARIA-HIDDEN=\"true\" Role=\"img\">x</span>\n",
			want: []finding{
				{kind: kindDisallowedAttribute, tag: "span"},
				{kind: kindDisallowedAttribute, tag: "span"},
			},
		},
		{
			name: "attribute wrapped across lines",
			src:  "<div\n  class=\"tabs\"\n  role=\"tablist\">\n\ncontent\n\n</div>\n",
			want: []finding{{kind: kindDisallowedAttribute, tag: "div"}},
		},
		{
			name: "allowed attributes pass",
			src:  "<a href=\"x\" id=\"y\" class=\"z\" title=\"t\" target=\"_blank\">x</a> <img src=\"a.png\" alt=\"\" width=\"10\">\n",
			want: nil,
		},
		{
			// data-role, data-aria-label, and language only resemble guarded names.
			name: "lookalike attribute names pass",
			src:  "<span data-role=\"x\" data-aria-label=\"y\" language=\"z\">x</span>\n",
			want: nil,
		},
		{
			name: "elements in fenced code are ignored",
			src:  "```html\n<iframe src=\"x\"></iframe>\n<div role=\"button\"></div>\n```\n",
			want: nil,
		},
		{
			name: "elements in inline code are ignored",
			src:  "Avoid `<button aria-label=\"x\">` in docs.\n",
			want: nil,
		},
		{
			name: "elements in html comments are ignored",
			src:  "<!-- <iframe src=\"x\" role=\"y\"></iframe> -->\n",
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

func TestDisallowedFindingMessage(t *testing.T) {
	t.Parallel()

	got := checkSource([]byte("# Page\n\n<span aria-label=\"x\">y</span>\n"))
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %+v", got)
	}
	if got[0].line != 3 {
		t.Errorf("want line 3, got %d", got[0].line)
	}
	if !strings.Contains(got[0].msg, "aria-label") {
		t.Errorf("message should name the attribute, got %q", got[0].msg)
	}
}

func TestRunDisallowed(t *testing.T) {
	t.Parallel()

	bad := filepath.Join(t.TempDir(), "bad.md")
	if err := os.WriteFile(bad, []byte("# Page\n\n<iframe src=\"x\" title=\"Demo\"></iframe>\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	if code := run([]string{bad}, &stdout, &stderr); code != 1 {
		t.Errorf("want exit 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), canonicalPath(bad)+":3: disallowed-element:") {
		t.Errorf("want a disallowed-element line, got:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "disallowed element(s) or attribute(s)") {
		t.Errorf("want the guard summary footer, got:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "invalid inline HTML issue(s)") {
		t.Errorf("guard findings should not print the inline HTML footer, got:\n%s", stderr.String())
	}
}
