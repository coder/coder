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
