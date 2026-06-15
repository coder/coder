package chattool

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func TestStripReadmeFrontmatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "LeadingFenceStripped",
			input: "---\ndisplay_name: Foo\ntags: [a, b]\n---\n# Title\n\nBody.\n",
			want:  "# Title\n\nBody.\n",
		},
		{
			name:  "NoFenceUnchanged",
			input: "# Title\n\nNo frontmatter here.\n",
			want:  "# Title\n\nNo frontmatter here.\n",
		},
		{
			name:  "FrontmatterOnlyLeavesBlank",
			input: "---\ndisplay_name: Foo\n---\n",
			want:  "",
		},
		{
			name:  "LeadingBlankLinesBeforeFence",
			input: "\n\n---\nkey: val\n---\nBody.\n",
			want:  "Body.\n",
		},
		{
			name:  "CRLFFence",
			input: "---\r\nkey: val\r\n---\r\n# Title\r\n",
			want:  "# Title\r\n",
		},
		{
			name:  "BOMPrefix",
			input: "\ufeff---\nkey: val\n---\nBody.\n",
			want:  "Body.\n",
		},
		{
			name:  "FenceWithTrailingWhitespace",
			input: "--- \nkey: val\n---\t\nBody.\n",
			want:  "Body.\n",
		},
		{
			name:  "UnterminatedFenceUnchanged",
			input: "---\nkey: val\nno closing fence\n",
			want:  "---\nkey: val\nno closing fence\n",
		},
		{
			name:  "BOMWithoutFenceStripsBOM",
			input: "\ufeff# Title\n\nNo frontmatter here.\n",
			want:  "# Title\n\nNo frontmatter here.\n",
		},
		{
			name:  "BOMWithUnterminatedFenceStripsBOM",
			input: "\ufeff---\nkey: val\nno closing fence\n",
			want:  "---\nkey: val\nno closing fence\n",
		},
		{
			name:  "HorizontalRuleNotFence",
			input: "# Title\n\n---\n\nMore.\n",
			want:  "# Title\n\n---\n\nMore.\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, stripReadmeFrontmatter(tc.input))
		})
	}
}

func TestExtractReadmeProse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "LinkTextKeptUrlDropped",
			input: "Use [Coder workspaces](https://coder.com/docs) now.",
			want:  "Use Coder workspaces now.",
		},
		{
			name:  "ImageDropped",
			input: "# T\n\n![Architecture](./a.svg)\n\nBody.",
			want:  "T Body.",
		},
		{
			name:  "HtmlCommentDropped",
			input: "Body.\n\n<!-- TODO: screenshot -->\n\nMore.",
			want:  "Body. More.",
		},
		{
			name:  "RawHtmlBlockDropped",
			input: "<div align=\"center\">\n<img src=\"x.png\">\n</div>\n\nReal prose.",
			want:  "Real prose.",
		},
		{
			name:  "CodeBlockDropped",
			input: "Intro.\n\n```sh\nrm -rf /\n```\n\nOutro.",
			want:  "Intro. Outro.",
		},
		{
			name:  "TableDropped",
			input: "Before.\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\nAfter.",
			want:  "Before. After.",
		},
		{
			name:  "HeadingAndParagraphOrder",
			input: "# Title\n\nLead.\n\n## Prereq\n\nDetail.",
			want:  "Title Lead. Prereq Detail.",
		},
		{
			name:  "EmphasisAndCodeSpanFlattened",
			input: "Run `make` for **speed**.",
			want:  "Run make for speed.",
		},
		{
			name:  "ListItemsJoined",
			input: "- one\n- two\n- three\n",
			want:  "one two three",
		},
		{
			name:  "EmptyReturnsEmpty",
			input: "",
			want:  "",
		},
		{
			name:  "ImageOnlyReturnsEmpty",
			input: "![x](y.png)\n",
			want:  "",
		},
		{
			name:  "HtmlOnlyReturnsEmpty",
			input: "<!-- just a comment -->\n",
			want:  "",
		},
		{
			// Mirrors a real template README opening (docker): H1 + lead prose with
			// an inline doc link, a TODO comment, and a section heading.
			name: "RealDockerOpening",
			input: "# Remote Development on Docker Containers\n\n" +
				"Provision Docker containers as [Coder workspaces](https://coder.com/docs/user-guides/workspace-management) with this example template.\n\n" +
				"<!-- TODO: Add screenshot -->\n\n## Prerequisites\n",
			want: "Remote Development on Docker Containers Provision Docker containers as Coder workspaces with this example template. Prerequisites",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, extractReadmeProse(readmeParser, tc.input))
		})
	}
}

// panickingParser implements parser.Parser and always panics, to exercise the
// recover guard in extractReadmeProse.
type panickingParser struct{}

func (panickingParser) Parse(text.Reader, ...parser.ParseOption) ast.Node { panic("boom") }
func (panickingParser) AddOptions(...parser.Option)                       {}

func TestExtractReadmeProse_RecoversFromPanic(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", extractReadmeProse(panickingParser{}, "# Title\n\nBody."))
}

func TestReadmeExcerpt(t *testing.T) {
	t.Parallel()

	t.Run("StripsFrontmatterAndTrims", func(t *testing.T) {
		t.Parallel()
		got := readmeExcerpt("---\nkey: val\n---\n\n  Body prose.  \n")
		require.Equal(t, "Body prose.", got)
	})

	t.Run("BlankReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "", readmeExcerpt(" \n\t\n"))
		require.Equal(t, "", readmeExcerpt("---\nkey: val\n---\n"))
	})

	t.Run("ExactlyAtCapNoEllipsis", func(t *testing.T) {
		t.Parallel()
		body := strings.Repeat("a", ListTemplatesReadmeExcerptMaxRunes)
		got := readmeExcerpt(body)
		require.Equal(t, body, got)
		require.Len(t, []rune(got), ListTemplatesReadmeExcerptMaxRunes)
	})

	t.Run("OverCapAddsEllipsis", func(t *testing.T) {
		t.Parallel()
		body := strings.Repeat("a", ListTemplatesReadmeExcerptMaxRunes+50)
		got := readmeExcerpt(body)
		gotRunes := []rune(got)
		require.Len(t, gotRunes, ListTemplatesReadmeExcerptMaxRunes)
		require.Equal(t, '…', gotRunes[len(gotRunes)-1])
	})

	t.Run("MultiByteBoundary", func(t *testing.T) {
		t.Parallel()
		// Each rune is multi-byte; truncation must not split a rune.
		body := strings.Repeat("世", ListTemplatesReadmeExcerptMaxRunes+10)
		got := readmeExcerpt(body)
		gotRunes := []rune(got)
		require.Len(t, gotRunes, ListTemplatesReadmeExcerptMaxRunes)
		require.Equal(t, '…', gotRunes[len(gotRunes)-1])
		require.True(t, strings.HasPrefix(got, "世"))
	})
}
