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
		{
			// Safelink drops links with unsafe URI schemes so they cannot
			// render as a clickable anchor.
			name:     "Unsafe link scheme",
			input:    `[click](javascript:alert(1))`,
			expected: `<p><tt>click</tt></p>`,
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

func TestEscapeMarkdownLinks(t *testing.T) {
	t.Parallel()

	t.Run("NeutralizesLinks", func(t *testing.T) {
		t.Parallel()

		// Each of these would produce a live <a>/<img> if left unescaped,
		// including the bare-URL autolink which needs no [](): gomarkdown
		// autolinks any "scheme://" run.
		for _, payload := range []string{
			"Eve [Re-auth](https://coder-sso.evil.example)",
			"![pixel](https://coder-sso.evil.example/x.png)",
			"https://coder-sso.evil.example",
			"<https://coder-sso.evil.example>",
			"mailto:attacker@coder-sso.evil.example",
			"MAILTO:attacker@coder-sso.evil.example",
		} {
			rendered := render.HTMLFromMarkdown(render.EscapeMarkdownLinks(payload))
			require.NotContains(t, rendered, "<a ", "payload %q produced a link: %q", payload, rendered)
			require.NotContains(t, rendered, "<img", "payload %q produced an image: %q", payload, rendered)
			require.NotContains(t, rendered, "evil.example\"", "payload %q leaked a URL attribute: %q", payload, rendered)
		}
	})

	t.Run("PreservesBenignPunctuation", func(t *testing.T) {
		t.Parallel()

		// Values with only benign punctuation are left completely untouched by
		// the escaper (no numeric entities), so webhook Markdown stays clean.
		// This includes emphasis/code markers (only links are escaped), ':'
		// outside of a "scheme://" (e.g. timestamps), and '#'.
		for _, name := range []string{
			"Anne-Marie", "Smith, John", "José", "李雷", "v1.2.3", "R&D 100%",
			"15:04:00 UTC", "Jan 2 15:04 MST", "C# developer",
			"muddy_russell78", "My *Cool* App", "2*speed", "code: `x`",
			// Parentheses are not escaped (they cannot complete a link
			// without the "[...]" label, which is escaped).
			"Jane Doe (she/her)", "Bob (Jr.)",
			// A ':' followed by a space or preceded by a digit does not start
			// a scheme, so it is left untouched.
			"Note: hello", "TODO: fix bug", "ratio 3:2",
		} {
			require.Equal(t, name, render.EscapeMarkdownLinks(name),
				"benign value %q should not be modified", name)
		}
	})

	t.Run("RendersBenignNamesLiterally", func(t *testing.T) {
		t.Parallel()

		// Names containing link-forming punctuation are escaped but still
		// render to the literal characters through the Markdown renderers.
		// (The '&' in a name is rendered as the HTML entity &amp; by the HTML
		// renderer, which is correct HTML escaping, so those names are only
		// checked via the plaintext renderer below.)
		for _, name := range []string{"J. Doe (Jr.)", "Anne-Marie"} {
			html := render.HTMLFromMarkdown(render.EscapeMarkdownLinks(name))
			require.Equal(t, "<p>"+name+"</p>", html)
		}

		for _, name := range []string{"J. Doe (Jr.)", "Café (R&D) 100%", "Anne-Marie"} {
			plain, err := render.PlaintextFromMarkdown(render.EscapeMarkdownLinks(name))
			require.NoError(t, err)
			require.Equal(t, name, plain)
		}
	})
}
