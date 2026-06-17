package chattool

import (
	"strings"
	"testing"

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
