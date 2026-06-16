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

	t.Run("BoundsHugeInput", func(t *testing.T) {
		t.Parallel()
		// Smoke test: a README far larger than the input bound still flows through
		// the whole pipeline within the cap. boundInput's branches are covered
		// directly by TestBoundInput.
		huge := "# Title\n\n" + strings.Repeat("word word word\n", readmeInputMaxBytes/8) // >64KiB, newlines throughout
		got := readmeText(huge, maxRunes)
		require.LessOrEqual(t, len([]rune(got)), maxRunes)
		require.NotContains(t, got, "<")
	})
}

func TestBoundInput(t *testing.T) {
	t.Parallel()

	t.Run("UnderCapUnchanged", func(t *testing.T) {
		t.Parallel()
		s := "short\nreadme\n"
		require.Equal(t, s, boundInput(s))
	})

	t.Run("NewlineBackoff", func(t *testing.T) {
		t.Parallel()
		// A newline within the cap: cut there, dropping the partial final line so
		// the parser never sees a fragment cut mid-line.
		head := strings.Repeat("a", readmeInputMaxBytes-3)
		got := boundInput(head + "\n" + strings.Repeat("b", 100))
		require.Equal(t, head, got)
		require.NotContains(t, got, "b")
	})

	t.Run("NoNewlineHardCut", func(t *testing.T) {
		t.Parallel()
		// No newline in the first cap bytes: hard-cut at exactly the cap.
		got := boundInput(strings.Repeat("a", readmeInputMaxBytes+100))
		require.Len(t, got, readmeInputMaxBytes)
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
