package render_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/render"
)

func TestPlaintext(t *testing.T) {
	t.Parallel()
	t.Run("Simple", func(t *testing.T) {
		t.Parallel()

		mdDescription := `# Provide the machine image
See the [registry](https://container.registry.blah/namespace) for options.

![Minion](https://octodex.github.com/images/minion.png)

**This is bold text.**
__This is bold text.__
*This is italic text.*
> Blockquotes can also be nested.
~~Strikethrough.~~

1. Lorem ipsum dolor sit amet.
2. Consectetur adipiscing elit.
3. Integer molestie lorem at massa.

` + "`There are also code tags!`"

		expected := "Provide the machine image\nSee the registry (https://container.registry.blah/namespace) for options.\n\nMinion (https://octodex.github.com/images/minion.png)\n\nThis is bold text.\nThis is bold text.\nThis is italic text.\n\nBlockquotes can also be nested.\nStrikethrough.\n\n1. Lorem ipsum dolor sit amet.\n2. Consectetur adipiscing elit.\n3. Integer molestie lorem at massa.\n\nThere are also code tags!"

		stripped, err := render.PlaintextFromMarkdown(mdDescription)
		require.NoError(t, err)
		require.Equal(t, expected, stripped)
	})

	t.Run("Nothing changes", func(t *testing.T) {
		t.Parallel()

		nothingChanges := "This is a simple description, so nothing changes."

		stripped, err := render.PlaintextFromMarkdown(nothingChanges)
		require.NoError(t, err)
		require.Equal(t, nothingChanges, stripped)
	})
}

func TestHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple",
			input:    `**Coder** is in *early access* mode. To ~~register~~ request access, fill out [this form](https://internal.example.com). ***Thank you!***`,
			expected: `<p><strong>Coder</strong> is in <em>early access</em> mode. To <del>register</del> request access, fill out <a href="https://internal.example.com">this form</a>. <strong><em>Thank you!</em></strong></p>`,
		},
		{
			name:     "Tricky",
			input:    `**Cod*er** is in *early a**ccess** <img src="foobar">mode`,
			expected: `<p><strong>Cod*er</strong> is in *early a<strong>ccess</strong> mode</p>`,
		},
		{
			name:     "XSS",
			input:    `<p onclick="alert(\"omghax\")">Click here to get access!</p>?`,
			expected: `<p>Click here to get access!?</p>`,
		},
		{
			name:     "No Markdown tags",
			input:    "This is a simple description, so nothing changes.",
			expected: "<p>This is a simple description, so nothing changes.</p>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rendered := render.HTMLFromMarkdown(tt.input)
			require.Equal(t, tt.expected, rendered)
		})
	}
}

func TestInnerTextFromMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"LinkTextKeptUrlDropped", "Use [Coder](https://coder.com/docs) now.", "Use Coder now."},
		{"ImageDropped", "# T\n\n![alt](a.svg)\n\nBody.", "T Body."},
		{"BadgeDropped", "[![discord](shield.png)](https://discord.gg/x)\n\nReal.", "Real."},
		{"CodeBlockKept", "Intro.\n\n```sh\nsudo usermod -aG docker coder\n```\n\nOutro.", "Intro. sudo usermod -aG docker coder Outro."},
		{"TableCellsKept", "Before.\n\n| env | required |\n|---|---|\n| FOO | yes |\n\nAfter.", "Before. env required FOO yes After."},
		{"HtmlInnerTextKept", "<p>Important: needs GPU.</p>", "Important: needs GPU."},
		{
			// Markdown nested inside a block-level HTML wrapper must still be
			// parsed (CommonMark terminates the HTML block at the blank line):
			// nav links collapse to text, badges drop. Regresses the gomarkdown
			// behavior that leaked raw badge markdown with URLs.
			"MarkdownInsideHtmlBlock",
			"<div align=\"center\">\n  <img src=\"logo.png\" alt=\"Logo\">\n</div>\n\n" +
				"[Docs](https://x.com/docs) | [Why](https://x.com/why)\n\n" +
				"[![badge](https://img.shields.io/x.svg)](https://x.com)\n\nReal prose.",
			"Docs | Why Real prose.",
		},
		{"ScriptDropped", "Before.\n\n<script>alert('x')</script>\n\nAfter.", "Before. After."},
		{"StyleDropped", "Before.\n\n<style>.x{color:red}</style>\n\nAfter.", "Before. After."},
		{"EmphasisAndCodeSpanFlattened", "Run `make` for **speed**.", "Run make for speed."},
		{"HeadingParagraphOrder", "# Title\n\nLead.\n\n## Prereq\n\nDetail.", "Title Lead. Prereq Detail."},
		{"SmartPunctuationPreserved", `Use "quotes" and dashes -- like this.`, `Use "quotes" and dashes -- like this.`},
		{"EmptyReturnsEmpty", "", ""},
		{"WhitespaceReturnsEmpty", "   \n\t\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := render.InnerTextFromMarkdown(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.expected, got)
		})
	}
}
