package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckTables(t *testing.T) {
	t.Parallel()

	type want struct {
		line  int
		first string
	}
	cases := []struct {
		name string
		src  string
		want []want
	}{
		{
			name: "pipe table with an empty header row",
			src:  "Intro.\n\n| | |\n| --- | --- |\n| Type | `string` |\n",
			want: []want{{line: 3}, {line: 3}},
		},
		{
			name: "pipe table with an empty corner cell",
			src:  "|  | Auditor | Owner |\n| --- | --- | --- |\n| Read | yes | yes |\n",
			want: []want{{line: 1, first: "Auditor"}},
		},
		{
			name: "pipe table with header text in every cell",
			src:  "| Property | Value |\n| --- | --- |\n| Type | `string` |\n",
		},
		{
			name: "header text that is only inline code or a link counts",
			src:  "| `name` | [Docs](./x.md) |\n| --- | --- |\n| a | b |\n",
		},
		{
			name: "pipe table inside a fenced code block is ignored",
			src:  "```md\n| | |\n| --- | --- |\n| a | b |\n```\n",
		},
		{
			name: "html table with an empty th",
			src:  "<table>\n<tr><th></th><th>Coder Desktop</th></tr>\n<tr><td>a</td><td>b</td></tr>\n</table>\n",
			want: []want{{line: 2, first: "Coder Desktop"}},
		},
		{
			name: "html th holding only a non-breaking space",
			src:  "<table>\n<tr>\n<th>&nbsp;</th>\n<th scope=\"col\">Name</th>\n</tr>\n</table>\n",
			want: []want{{line: 3, first: "Name"}},
		},
		{
			name: "html th holding only a line break",
			src:  "<table><tr><th><br></th><th>Name</th></tr></table>\n",
			want: []want{{line: 1, first: "Name"}},
		},
		{
			name: "html th with nested markup and text",
			src:  "<table>\n<tr><th scope=\"col\"><strong>Feature</strong></th><th>Name</th></tr>\n</table>\n",
		},
		{
			name: "html th whose text is Markdown between html blocks",
			src:  "<table>\n<tr>\n<th>\n\n**Feature**\n\n</th>\n</tr>\n</table>\n",
		},
		{
			name: "html table inside a fenced code block is ignored",
			src:  "```html\n<table><tr><th></th></tr></table>\n```\n",
		},
		{
			name: "pipe header holding only a non-breaking space entity",
			src:  "| &nbsp; | A |\n| --- | --- |\n| a | b |\n",
			want: []want{{line: 1, first: "A"}},
		},
		{
			name: "pipe header holding only a numeric non-breaking space entity",
			src:  "| &#160; | A |\n| --- | --- |\n| a | b |\n",
			want: []want{{line: 1, first: "A"}},
		},
		{
			name: "html th whose Markdown between blocks is only a non-breaking space",
			src:  "<table>\n<tr>\n<th>\n\n&nbsp;\n\n</th>\n<th>Name</th>\n</tr>\n</table>\n",
			want: []want{{line: 3, first: "Name"}},
		},
		{
			// A code span renders its content literally, so `&nbsp;` is the
			// six visible characters, not a space.
			name: "an entity inside a code span is text",
			src:  "| `&nbsp;` | A |\n| --- | --- |\n| a | b |\n",
		},
		{
			name: "html th holding only an image with alt text",
			src:  "<table><tr><th><img alt=\"Logo\" src=\"x.png\"></th></tr></table>\n",
		},
		{
			name: "html th holding only an image with empty alt text",
			src:  "<table><tr><th><img alt=\"\" src=\"x.png\"></th><th>Name</th></tr></table>\n",
			want: []want{{line: 1, first: "Name"}},
		},
		{
			name: "pipe header holding only a raw image with alt text",
			src:  "| <img alt=\"Logo\" src=\"x.png\"> | A |\n| --- | --- |\n| a | b |\n",
		},
		{
			name: "pipe header holding only a raw image with empty alt text",
			src:  "| <img alt=\"\" src=\"x.png\"> | A |\n| --- | --- |\n| a | b |\n",
			want: []want{{line: 1, first: "A"}},
		},
		{
			name: "html th whose Markdown between blocks is an autolink",
			src:  "<table>\n<tr>\n<th>\n\n<https://coder.com>\n\n</th>\n</tr>\n</table>\n",
		},
		{
			name: "html th whose Markdown between blocks is a fenced code block",
			src:  "<table>\n<tr>\n<th>\n\n```sh\ncoder login\n```\n\n</th>\n</tr>\n</table>\n",
		},
		{
			name: "every header empty names no first header",
			src:  "<table><tr><th></th></tr></table>\n",
			want: []want{{line: 1}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := checkTables([]byte(tc.src))
			if len(got) != len(tc.want) {
				t.Fatalf("want %d findings, got %d: %+v", len(tc.want), len(got), got)
			}
			for i, w := range tc.want {
				f := got[i]
				if f.kind != kindEmptyTableHeader {
					t.Errorf("finding %d: want kind %s, got %s", i, kindEmptyTableHeader, f.kind)
				}
				if f.line != w.line {
					t.Errorf("finding %d: want line %d, got %d", i, w.line, f.line)
				}
				firstHeader := "(none)"
				if w.first != "" {
					firstHeader = `"` + w.first + `"`
				}
				if !strings.Contains(f.msg, "first header "+firstHeader) {
					t.Errorf("finding %d: want message naming first header %s, got %q", i, firstHeader, f.msg)
				}
			}
		})
	}
}

func TestRunEmptyTableHeader(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.md")
	src := "# Page\n\n|  | Auditor |\n| --- | --- |\n| Read | yes |\n"
	if err := os.WriteFile(bad, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	if code := run([]string{bad}, &stdout, &stderr); code != 1 {
		t.Errorf("want exit 1, got %d", code)
	}
	wantLine := canonicalPath(bad) + `:3: empty-table-header: header cell 1 has no text (table's first header "Auditor")`
	if !strings.Contains(stdout.String(), wantLine) {
		t.Errorf("want %q in output, got:\n%s", wantLine, stdout.String())
	}
	if !strings.Contains(stderr.String(), "empty table header cell(s)") {
		t.Errorf("want the table summary footer, got:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "invalid inline HTML issue(s)") {
		t.Errorf("table findings should not print the inline HTML footer, got:\n%s", stderr.String())
	}
}
