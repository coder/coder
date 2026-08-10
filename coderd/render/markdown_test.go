package render_test

import (
	"strings"
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
		{"ImageDropped", "# T\n\n![alt](a.svg)\n\nBody.", "T\nBody."},
		{"BadgeDropped", "[![discord](shield.png)](https://discord.gg/x)\n\nReal.", "Real."},
		{"CodeBlockLinesKept", "Intro.\n\n```sh\nnpm install\nnpm run dev\n```\n\nOutro.", "Intro.\nnpm install\nnpm run dev\nOutro."},
		{"TableCellsKept", "Before.\n\n| env | required |\n|---|---|\n| FOO | yes |\n\nAfter.", "Before.\nenv\nrequired\nFOO\nyes\nAfter."},
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
			"Docs | Why\nReal prose.",
		},
		{"ScriptDropped", "Before.\n\n<script>alert('x')</script>\n\nAfter.", "Before.\nAfter."},
		// An empty-body <script src=...> must not leave the skip armed and eat the
		// next text run (guards the skipNextText reset on non-text tokens).
		{"ScriptSrcEmptyBody", "Before.\n\n<script src=\"x.js\"></script>\n\nAfter.", "Before.\nAfter."},
		{"StyleDropped", "Before.\n\n<style>.x{color:red}</style>\n\nAfter.", "Before.\nAfter."},
		// A bare </script> in prose must not underflow the skip and swallow what
		// follows.
		{"BareScriptCloseNoUnderflow", "Before.\n\n</script>\n\nAfter.", "Before.\nAfter."},
		// An unterminated raw-text element is, per the HTML spec, a single run to
		// EOF, so the remainder is unavoidably consumed; it must not error.
		{"UnterminatedScriptEatsRest", "Intro.\n\n<script>\nvar x = 1;\n\nMore prose.", "Intro."},
		{"EmphasisAndCodeSpanFlattened", "Run `make` for **speed**.", "Run make for speed."},
		{"HeadingParagraphOrder", "# Title\n\nLead.\n\n## Prereq\n\nDetail.", "Title\nLead.\nPrereq\nDetail."},
		// Straight ASCII punctuation must stay ASCII (goldmark applies no
		// Typographer), and existing smart punctuation passes through unchanged.
		// The smart characters are \u-escaped so the docs linter does not rewrite
		// them back to ASCII in source.
		{"PunctuationNotRewritten", "Range 10\u201420, \"q\", ... and smart \u201cq\u201d \u2014 \u2026", "Range 10\u201420, \"q\", ... and smart \u201cq\u201d \u2014 \u2026"},
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

func TestHTMLFromMarkdownSafe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		contains []string
		absent   []string
	}{
		{
			name:     "explicit https link preserved",
			input:    "[Coder](https://coder.com)",
			contains: []string{`<a href="https://coder.com">Coder</a>`},
		},
		{
			name:     "explicit http link preserved",
			input:    "[Link](http://example.com)",
			contains: []string{`<a href="http://example.com">Link</a>`},
		},
		{
			name:   "javascript URI blocked by Safelink",
			input:  "[Click](javascript:alert(1))",
			absent: []string{"javascript:", "<a href"},
		},
		{
			name:   "data URI blocked by Safelink",
			input:  "[Click](data:text/html,<script>alert(1)</script>)",
			absent: []string{"data:", "<a href"},
		},
		{
			name:     "bare URL NOT auto-linked",
			input:    "Visit https://evil.example for details",
			absent:   []string{"<a href"},
			contains: []string{"https://evil.example"},
		},
		{
			name:     "bold and emphasis still work",
			input:    "**bold** and *italic*",
			contains: []string{"<strong>bold</strong>", "<em>italic</em>"},
		},
		{
			name:   "raw HTML stripped",
			input:  `<script>alert(1)</script>`,
			absent: []string{"<script>"},
		},
		{
			name:     "escaped markdown renders as literal text",
			input:    `\[not a link\]\(https://evil.example\)`,
			absent:   []string{"<a href"},
			contains: []string{"[not a link]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := render.HTMLFromMarkdownSafe(tt.input)
			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("output %q should contain %q", result, s)
				}
			}
			for _, s := range tt.absent {
				if strings.Contains(result, s) {
					t.Errorf("output %q should NOT contain %q", result, s)
				}
			}
		})
	}
}

func TestHTMLFromMarkdownSafelink(t *testing.T) {
	t.Parallel()

	// Safelink is scoped to HTMLFromMarkdownSafe. HTMLFromMarkdown renders
	// admin-authored deployment text and keeps its pre-existing behavior, so
	// it does not block or rewrite link schemes.
	t.Run("HTMLFromMarkdownSafe blocks javascript URI", func(t *testing.T) {
		t.Parallel()
		result := render.HTMLFromMarkdownSafe("[Click](javascript:alert(1))")
		if strings.Contains(result, "javascript:") {
			t.Error("HTMLFromMarkdownSafe should block javascript: URIs via Safelink")
		}
	})

	t.Run("HTMLFromMarkdown does not rewrite custom-scheme links", func(t *testing.T) {
		t.Parallel()
		// Admin-authored deployment text may legitimately use custom schemes.
		result := render.HTMLFromMarkdown("[Open the app](slack://channel)")
		if !strings.Contains(result, `href="slack://channel"`) {
			t.Errorf("HTMLFromMarkdown should render custom-scheme links, got %q", result)
		}
	})

	t.Run("HTMLFromMarkdown allows autolinks", func(t *testing.T) {
		t.Parallel()
		result := render.HTMLFromMarkdown("Visit https://coder.com for details")
		if !strings.Contains(result, "<a href") {
			t.Error("HTMLFromMarkdown should auto-link bare URLs")
		}
	})

	t.Run("HTMLFromMarkdownSafe disables autolinks", func(t *testing.T) {
		t.Parallel()
		result := render.HTMLFromMarkdownSafe("Visit https://coder.com for details")
		if strings.Contains(result, "<a href") {
			t.Error("HTMLFromMarkdownSafe should NOT auto-link bare URLs")
		}
	})
}
