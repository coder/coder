package chattool

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
