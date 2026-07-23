package chattool

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
)

func TestReadmeText(t *testing.T) {
	t.Parallel()

	const maxRunes = 1000

	t.Run("StripsFrontmatterThenRenders", func(t *testing.T) {
		t.Parallel()
		got := readmeText("---\nkey: val\n---\n\n# Title\n\nBody prose.\n", maxRunes)
		require.Equal(t, "Title\nBody prose.", got)
	})

	t.Run("BlankReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "", readmeText(" \n\t\n", maxRunes))
		require.Equal(t, "", readmeText("---\nkey: val\n---\n", maxRunes)) // frontmatter only
	})

	t.Run("OverCapAddsEllipsis", func(t *testing.T) {
		t.Parallel()
		got := readmeText(strings.Repeat("a", maxRunes+50), maxRunes)
		gotRunes := []rune(got)
		require.Len(t, gotRunes, maxRunes)
		require.Equal(t, '…', gotRunes[len(gotRunes)-1])
	})

	t.Run("CapsParseInput", func(t *testing.T) {
		t.Parallel()
		// A rune budget far larger than the input makes the byte cap the only
		// thing bounding the output, so this fails if readmeText stops capping the
		// parse input.
		huge := strings.Repeat("word ", 40_000) // ~200KiB of prose
		got := readmeText(huge, 10_000_000)
		require.NotEmpty(t, got)
		require.LessOrEqual(t, len(got), readmeInputMaxBytes)
	})

	t.Run("RawTextElementSpanningCutRendersEmpty", func(t *testing.T) {
		t.Parallel()
		// A <style> that opens before the cap and closes after it is left
		// unterminated by truncation, so its raw-text run swallows the document and
		// the excerpt is empty. The same content under the cap renders fine, which
		// isolates the mid-cut as the cause.
		css := strings.Repeat("  .x { color: red; }\n", readmeInputMaxBytes/10) // > cap
		require.Equal(t, "", readmeText("<style>\n"+css+"</style>\n\nReal prose.\n", maxRunes))
		require.Equal(t, "Real prose.", readmeText("<style>\n.x{}\n</style>\n\nReal prose.\n", maxRunes))
	})
}

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
			name:  "NoFence",
			input: "\n# Title\n\nNo frontmatter here.\n",
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
			want:  "# Title\n",
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
		{
			// Only the leading block is frontmatter. A "---" thematic break in
			// the body must survive verbatim, not re-open a fence.
			name:  "FrontmatterThenThematicBreakInBody",
			input: "---\ntitle: Foo\n---\n# Heading\n\n---\n\nSecond section.\n",
			want:  "# Heading\n\n---\n\nSecond section.\n",
		},
		{
			// Multiple body breaks: every "---" after the closing fence is body
			// content. An even count must not silently eat the sections between.
			name:  "FrontmatterThenMultipleThematicBreaks",
			input: "---\ntitle: Foo\n---\nIntro\n\n---\n\nMid\n\n---\n\nEnd\n",
			want:  "Intro\n\n---\n\nMid\n\n---\n\nEnd\n",
		},
		{
			// An indented "---" is not a YAML document separator (must be at
			// column 0), so it stays inside the frontmatter block.
			name:  "IndentedFenceInFrontmatterNotClosing",
			input: "---\nkey: val\n  ---\nstill: fm\n---\nBody.\n",
			want:  "Body.\n",
		},
		{
			// Final body line without a trailing newline is still emitted and
			// newline-terminated.
			name:  "BodyWithoutTrailingNewline",
			input: "---\nk: v\n---\nBody no newline",
			want:  "Body no newline\n",
		},
		{
			name:  "WhitespaceOnlyReturnsEmpty",
			input: "   \n\t\n",
			want:  "",
		},
		{
			name:  "EmptyReturnsEmpty",
			input: "",
			want:  "",
		},
		{
			// Leading indentation marks a Markdown code block. It must survive
			// verbatim; per-line trimming would turn the block into a paragraph.
			name:  "IndentedCodeBlockPreserved",
			input: "---\ntitle: Foo\n---\n# Heading\n\n    indented := code\n    more := code\n",
			want:  "# Heading\n\n    indented := code\n    more := code\n",
		},
		{
			// Tabs and spaces inside a fenced code block are content, not
			// stray whitespace, and must not be stripped.
			name:  "FencedCodeBlockPreservesIndentation",
			input: "---\ntitle: Foo\n---\n```go\nfunc main() {\n\tprintln(\"hi\")\n}\n```\n",
			want:  "```go\nfunc main() {\n\tprintln(\"hi\")\n}\n```\n",
		},
		{
			// A "---" inside a body code block is literal content, not a fence,
			// once the leading frontmatter block has closed.
			name:  "ThematicBreakInsideBodyCodeBlock",
			input: "---\ntitle: Foo\n---\n```\n---\n```\nAfter.\n",
			want:  "```\n---\n```\nAfter.\n",
		},
		{
			// Table pipes and the dashed alignment row pass through unchanged;
			// the alignment row is not a frontmatter fence.
			name:  "TablePreserved",
			input: "---\ntitle: Foo\n---\n| Col A | Col B |\n| ----- | ----- |\n| 1     | 2     |\n",
			want:  "| Col A | Col B |\n| ----- | ----- |\n| 1     | 2     |\n",
		},
		{
			name:  "Oversized",
			input: strings.Repeat("x", readmeInputMaxBytes+2),
			want:  strings.Repeat("x", readmeInputMaxBytes+2),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := stripReadmeFrontmatter(tc.input)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("unexpected diff: %s", diff)
			}
		})
	}
}
