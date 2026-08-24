package render

import (
	"strings"
	"testing"

	"github.com/gomarkdown/markdown/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// suspendedBody mirrors the live TemplateUserAccountSuspended body: the
// untrusted value sits mid-paragraph with trusted text on both sides.
func suspendedBody(value string) string {
	return "The account belongs to **" + value + "** and it was suspended by **rob**."
}

// TestEscapeMarkdownFenceInfo covers the sink that made backtick inlineCritical.
//
// gomarkdown writes a fenced block's info string into class="language-..."
// without escaping (html/renderer.go, appendLanguageAttr). html.SkipHTML does
// not apply because the node is a CodeBlock rather than an HTMLBlock, so a "in
// the info string closes the attribute and a > closes the tag. That makes the
// info string a live HTML sink, which is why backtick cannot be treated as an
// emphasis character that "carries no destination".
func TestEscapeMarkdownFenceInfo(t *testing.T) {
	t.Parallel()

	const info = `"><a/href="https://attacker.example/login">Click here`

	// The payload needs a line after the closing fence. Without one the
	// template's trailing text lands on the closing fence's line, the fence
	// stops being a fence, and the case passes for the wrong reason.
	for name, value := range map[string]string{
		"Anchor":  "Eve\n\n```" + info + "\nhidden\n```\nmore",
		"Image":   "Eve\n\n```\"><img/src=x/onerror=alert(1)>\nhidden\n```\nmore",
		"Tilde":   "Eve\n\n~~~" + info + "\nhidden\n~~~\nmore",
		"AtStart": "```" + info + "\nhidden\n```\nmore",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			html := HTMLFromNotificationMarkdown(suspendedBody(EscapeMarkdown(value)))
			assert.NotContains(t, html, `class="language-`,
				"an info string reached the class attribute: %s", html)
			assert.NotContains(t, html, "<a/", "the info string produced an anchor: %s", html)
			assert.NotContains(t, html, "<img/", "the info string produced an image: %s", html)
		})
	}

	// Liveness: the anchor payload must actually produce the sink unescaped,
	// or the assertions above guard nothing.
	raw := HTMLFromNotificationMarkdown(suspendedBody("Eve\n\n```" + info + "\nhidden\n```\nmore"))
	require.Contains(t, raw, `class="language-`,
		"vacuous test: the payload no longer reaches the info-string sink even unescaped")
}

// TestEscapeMarkdownColon covers ":", which opens a definition list and also a
// GFM table delimiter row. Escaping "|" does not reach the table case, because
// a delimiter row's pipes are mid-line and "|" is escaped only in leading
// position. ":" is folded rather than escaped because glamour does not honor
// "\:", which would leak a backslash into the plaintext part.
//
// Rendered under parser.CommonExtensions rather than notificationExtensions on
// purpose. Both spellings are also dead because notificationExtensions no
// longer enables Tables or DefinitionLists, so rendering through the shipped
// config would pass whether or not the escaper does anything. Asserting against
// the permissive config keeps this a test of EscapeMarkdown, and keeps the two
// defenses independent: re-enabling either extension must not reopen the gap.
func TestEscapeMarkdownColon(t *testing.T) {
	t.Parallel()

	const permissive = parser.CommonExtensions | parser.HardLineBreak

	// A single-column table needs pipes on the delimiter row, so there is no
	// pipeless spelling to cover here.
	for name, value := range map[string]string{
		"DefinitionList":   "Term\n: definition",
		"TableColonBoth":   "a | b\n:-- | --:\nc | d",
		"TableColonCentre": "a | b\n:-: | :-:\nc | d",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			escaped := EscapeMarkdown(value)
			html := renderHTML(suspendedBody(escaped), permissive)
			plain, err := PlaintextFromMarkdown(suspendedBody(escaped))
			require.NoError(t, err)

			for _, tag := range []string{"<table", "<dl", "<dt", "<dd"} {
				assert.NotContains(t, html, tag, "value %q rendered %s", value, html)
			}
			assert.NotContains(t, plain, `\`, "folding ':' should not leak a backslash: %q", plain)

			// Liveness: unescaped, each value must actually produce one of
			// those tags, or the assertions above guard nothing.
			raw := renderHTML(suspendedBody(value), permissive)
			assert.True(t,
				strings.Contains(raw, "<table") || strings.Contains(raw, "<dl"),
				"vacuous row: %q produces no table or definition list even unescaped", value)
		})
	}
}

// TestNotificationExtensionsDropUnusedGrammar pins the allowlist. Each
// construct below is reachable from an untrusted value and used by no shipped
// template, so the parser should not recognize it at all.
func TestNotificationExtensionsDropUnusedGrammar(t *testing.T) {
	t.Parallel()

	const permissive = parser.CommonExtensions | parser.HardLineBreak

	for name, tc := range map[string]struct{ markdown, tag string }{
		"Tables":          {"a | b\n:-- | --:\nc | d", "<table"},
		"DefinitionLists": {"Term\n: definition", "<dl"},
		"MathJax":         {"Eve $x^2$ end", `class="math`},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Contains(t, renderHTML(tc.markdown, permissive), tc.tag,
				"the construct is no longer reachable under CommonExtensions, so this test guards nothing")
			assert.NotContains(t, HTMLFromNotificationMarkdown(tc.markdown), tc.tag,
				"notificationExtensions still enables %s", name)
		})
	}

	// What shipped templates do use must keep rendering.
	for _, tc := range []struct{ markdown, want string }{
		{"see [the docs](https://coder.com/docs/x).", `<a href="https://coder.com/docs/x"`},
		{"Your workspace **foo** was suspended.", "<strong>foo</strong>"},
		{"Resources:\n\n- one\n- two\n", "<li>"},
		{"marked as [**dormant**](https://coder.com/docs/y) because", `<a href="https://coder.com/docs/y"`},
	} {
		assert.Contains(t, HTMLFromNotificationMarkdown(tc.markdown), tc.want)
	}
}

// TestEscapeMarkdownIndentedCode covers the one block construct with no escape:
// four leading spaces open an indented code block and a space cannot be
// backslash-escaped, so the run is truncated to maxLeadingSpaces instead.
func TestEscapeMarkdownIndentedCode(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]string{
		"FourSpaces":   "Eve\n\n    hidden",
		"EightSpaces":  "Eve\n\n        hidden",
		"SingleBreak":  "Eve\n    hidden",
		"DeepInList":   "Eve\n\n        - hidden",
		"OnlyIndented": "    hidden",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			html := HTMLFromNotificationMarkdown(suspendedBody(EscapeMarkdown(value)))
			assert.NotContains(t, html, "<pre", "value %q produced a code block: %s", value, html)
		})
	}

	// Indentation up to the cap is preserved, so the documented multi-line
	// custom notification values keep their shape.
	require.Equal(t, "Test results:\n  • ok", EscapeMarkdown("Test results:\n  • ok"))
	require.Equal(t, "a\n   b", EscapeMarkdown("a\n   b"))
	require.Equal(t, "a\n   b", EscapeMarkdown("a\n     b"))
}

// TestEscapeMarkdownEmptyLinkDestination covers the panic html.Safelink
// introduced. parser.IsSafeURL slices a destination to each candidate prefix
// length before checking the destination is that long, so an empty destination
// with no spare capacity reads out of range.
func TestEscapeMarkdownEmptyLinkDestination(t *testing.T) {
	t.Parallel()

	for _, md := range []string{
		"[our docs]()", "![px]()", "[a]( )", "[](https://coder.com)", "[a](x)",
	} {
		assert.NotPanics(t, func() { _ = HTMLFromNotificationMarkdown(md) }, "markdown %q", md)
		// The shared renderer is reachable outside notifications, via
		// OIDCConfig.SignupsDisabledText.
		assert.NotPanics(t, func() { _ = HTMLFromMarkdown(md) }, "markdown %q", md)
	}

	// Destinations Safelink still permits must keep rendering.
	for _, tc := range []struct{ md, want string }{
		{"[a](https://coder.com)", `<a href="https://coder.com"`},
		{"[a](/path)", `<a href="/path"`},
		{"[a](./p)", `<a href="./p"`},
		{"[a](mailto:x@y.z)", `<a href="mailto:x@y.z"`},
	} {
		assert.Contains(t, HTMLFromNotificationMarkdown(tc.md), tc.want)
	}

	// And the ones it rejects must stay rejected.
	for _, md := range []string{"[a](javascript:alert(1))", "[a](data:text/html;base64,eA==)"} {
		assert.NotContains(t, HTMLFromNotificationMarkdown(md), "<a ", "unsafe scheme linked: %s", md)
	}
}

// TestEscapeMarkdownResiduals pins the two gaps EscapeMarkdown cannot close, so
// that closing either one later is a deliberate change rather than an accident,
// and so the limits stay visible next to the function that has them.
//
// Both depend on where the value lands rather than on what it contains, which
// is exactly what a pre-render escaper cannot see.
func TestEscapeMarkdownResiduals(t *testing.T) {
	t.Parallel()

	t.Run("CodeSpanSwallowsEscapes", func(t *testing.T) {
		t.Parallel()

		// CommonMark does not process escapes inside a code span, so a template
		// that wraps the value in backticks renders our backslashes literally.
		// The workspace out-of-disk body does this with `{{$volume.path}}`.
		html := HTMLFromNotificationMarkdown("The volume `" + EscapeMarkdown("config[0]") + "` is full.")
		require.Contains(t, html, `config\[0\]`,
			"if this no longer leaks, the residual is closed and this test should become an assertion that it stays closed")
	})

	t.Run("FirstLineIsNotFolded", func(t *testing.T) {
		t.Parallel()

		// The fold can only remove line breaks EscapeMarkdown emitted, so a
		// value whose first line opens a fold construct is still in whatever
		// position the template gave it. No shipped template places a value at
		// a line start beneath a text line; this test is what fails if one does.
		html := HTMLFromNotificationMarkdown("Trusted line\n" + EscapeMarkdown("===\nx") + "\nTrusted trailer.")
		require.Contains(t, html, "<h1",
			"if this no longer promotes a heading, the residual is closed")
	})

	t.Run("TildeFenceBlanksATitle", func(t *testing.T) {
		t.Parallel()

		// The same first-line wall, reached through the title templates that
		// begin with a label ("{{.Labels.initiator}} shared a chat with you").
		// A value of "~~~" makes the whole title an unterminated tilde fence
		// whose info string is the trusted text, and glamour renders a code
		// block with no content, so the Subject, <title> and heading all come
		// out empty.
		//
		// Escaping "~" is not available: glamour does not honor "\~", so it
		// would leak a backslash into every plaintext part. Closing this needs
		// either a guard on an empty rendered subject or the placeholder
		// rewrite, not another character class.
		subject, err := PlaintextFromMarkdown(EscapeMarkdown("~~~") + " shared a chat with you")
		require.NoError(t, err)
		require.Empty(t, subject,
			"if this no longer blanks the subject, the residual is closed")

		// The backtick spelling of the same trick is closed, because backtick
		// is escaped everywhere.
		subject, err = PlaintextFromMarkdown(EscapeMarkdown("```") + " shared a chat with you")
		require.NoError(t, err)
		require.Equal(t, "``` shared a chat with you", subject)
	})
}

// TestEscapeMarkdownNoStrayBackslash asserts the escaper's own invariant across
// every interpolation position a shipped template provides, not just the
// mid-paragraph one. The assertion already existed; the positions did not, and
// that is why the code-span residual above went unnoticed.
func TestEscapeMarkdownNoStrayBackslash(t *testing.T) {
	t.Parallel()

	positions := map[string]func(string) string{
		"midline":    suspendedBody,
		"linestart":  func(v string) string { return v + " shared a chat with you." },
		"afterblank": func(v string) string { return "Hi.\n\n" + v + "\n\nRegards." },
		"listitem":   func(v string) string { return "Resources:\n\n- " + v + "\n" },
		"trailing":   func(v string) string { return "The account belongs to **" + v + "**" },
	}

	for _, value := range []string{
		"William Tables", "bobby-workspace", "Bobby's Template",
		"O'Brien-Smith (Eng) 100%", "José Müller 日本語",
		"config[0]", "vol(1)", "1.5", "user_override",
	} {
		for name, pos := range positions {
			t.Run(name+"/"+value, func(t *testing.T) {
				t.Parallel()

				md := pos(EscapeMarkdown(value))
				html := HTMLFromNotificationMarkdown(md)
				plain, err := PlaintextFromMarkdown(md)
				require.NoError(t, err)

				assert.NotContains(t, html, `\`, "stray backslash in HTML: %s", html)
				assert.NotContains(t, plain, `\`, "stray backslash in plaintext: %q", plain)
				assert.False(t, strings.Contains(html, `class="language-`),
					"benign value reached the info-string sink: %s", html)
			})
		}
	}
}
