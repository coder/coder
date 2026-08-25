package render

import (
	"strings"
	"testing"

	"github.com/gomarkdown/markdown/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xhtml "golang.org/x/net/html"
)

// permissive is the grammar notificationExtensions used to enable. Tests that
// must exercise the escaper rather than the allowlist render against it.
const permissive = parser.CommonExtensions | parser.HardLineBreak

// suspendedBody mirrors the live TemplateUserAccountSuspended body: the
// untrusted value sits mid-paragraph with trusted text on both sides.
func suspendedBody(value string) string {
	return "The account belongs to **" + value + "** and it was suspended by **rob**."
}

// TestEscapeMarkdownFenceInfo covers the sink that made backtick inlineCritical
// (gomarkdown html/renderer.go, appendLanguageAttr): a `"` in a fence's info
// string closes the class attribute and a `>` closes the tag.
func TestEscapeMarkdownFenceInfo(t *testing.T) {
	t.Parallel()

	const info = `"><a/href="https://attacker.example/login">Click here`

	// Each payload needs a line after the closing fence, or the template's
	// trailing text lands on it and the fence stops being one.
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

	// Liveness: unescaped, the anchor payload must reach the sink.
	raw := HTMLFromNotificationMarkdown(suspendedBody("Eve\n\n```" + info + "\nhidden\n```\nmore"))
	require.Contains(t, raw, `class="language-`,
		"vacuous test: the payload no longer reaches the info-string sink even unescaped")
}

// TestEscapeMarkdownColon covers ":", which opens a definition list and a GFM
// delimiter row. Rendered under CommonExtensions rather than the shipped
// allowlist, which already kills both constructs and so would pass whether or
// not the escaper did anything. This keeps the two defenses independent.
func TestEscapeMarkdownColon(t *testing.T) {
	t.Parallel()

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

			// Liveness: unescaped, each value must produce one of those tags.
			raw := renderHTML(suspendedBody(value), permissive)
			assert.True(t,
				strings.Contains(raw, "<table") || strings.Contains(raw, "<dl"),
				"vacuous row: %q produces no table or definition list even unescaped", value)
		})
	}
}

// TestNotificationExtensionsDropUnusedGrammar pins the allowlist: each construct
// is openable from an untrusted value and used by no shipped template.
func TestNotificationExtensionsDropUnusedGrammar(t *testing.T) {
	t.Parallel()

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

// TestEscapeMarkdownIndentedCode covers the one construct with no escape: a
// space cannot be backslash-escaped, so the run is truncated instead.
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

	// Indentation up to the cap is preserved, so documented multi-line custom
	// notification values keep their shape.
	require.Equal(t, "Test results:\n  • ok", EscapeMarkdown("Test results:\n  • ok"))
	require.Equal(t, "a\n   b", EscapeMarkdown("a\n   b"))
	require.Equal(t, "a\n   b", EscapeMarkdown("a\n     b"))
}

// TestEscapeMarkdownEmptyLinkDestination covers the panic html.Safelink
// introduced: parser.IsSafeURL slices a destination before bounds-checking it.
func TestEscapeMarkdownEmptyLinkDestination(t *testing.T) {
	t.Parallel()

	for _, md := range []string{
		"[our docs]()", "![px]()", "[a]( )", "[](https://coder.com)", "[a](x)",
	} {
		assert.NotPanics(t, func() { _ = HTMLFromNotificationMarkdown(md) }, "markdown %q", md)
		// Reachable outside notifications, via OIDCConfig.SignupsDisabledText.
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

	// Safelink also drops these two, a silent loss for a template author rather
	// than a security property. Pinned so renderHTML's comment cannot drift.
	for _, md := range []string{"[a](#anchor)", "[a](docs/x.md)"} {
		assert.NotContains(t, HTMLFromNotificationMarkdown(md), "<a ",
			"destination %q now renders an anchor; update the comment on renderHTML", md)
	}
}

// TestEscapeMarkdownLeadingFoldConstruct covers a value whose own first line is
// a fold construct, which the fold cannot reach and escaping must handle. A
// title beginning "~~~" is an unterminated fence swallowing the trusted text
// into an empty code block; "===" beneath a text line underlines it into an
// <h1>. Escaping costs a visible backslash, so the untouched cases below matter
// as much as the neutralized ones.
func TestEscapeMarkdownLeadingFoldConstruct(t *testing.T) {
	t.Parallel()

	t.Run("TitleKeepsItsTrustedText", func(t *testing.T) {
		t.Parallel()

		for _, value := range []string{"~~~", "~~~~", "~~~x", "~~~ ", "  ~~~"} {
			subject, err := PlaintextFromMarkdown(EscapeMarkdown(value) + " shared a chat with you")
			require.NoError(t, err)
			assert.Contains(t, subject, "shared a chat with you",
				"value %q swallowed the trusted subject text", value)
		}

		// The backtick spelling is closed by backtick being inlineCritical.
		subject, err := PlaintextFromMarkdown(EscapeMarkdown("```") + " shared a chat with you")
		require.NoError(t, err)
		require.Equal(t, "``` shared a chat with you", subject)
	})

	t.Run("SetextCannotPromoteATrustedLine", func(t *testing.T) {
		t.Parallel()

		for _, value := range []string{"===", "=", "===\nx", "  ===  "} {
			html := HTMLFromNotificationMarkdown(
				"Trusted line\n" + EscapeMarkdown(value) + "\nTrusted trailer.")
			assert.NotContains(t, html, "<h1", "value %q promoted a heading: %s", value, html)
		}
	})

	t.Run("NonConstructsAreUntouched", func(t *testing.T) {
		t.Parallel()

		// These begin with a fold character without being a construct; escaping
		// them would put a backslash in front of an ordinary display name.
		for _, value := range []string{
			"=> next", "~tilde name", "= x", "~~strike~~", "=?utf-8?q?x?=",
			"~", "~~", "=== and more", "a\n===",
		} {
			plain, err := PlaintextFromMarkdown(EscapeMarkdown(value) + " end")
			require.NoError(t, err)
			assert.NotContains(t, plain, `\`,
				"value %q was escaped when it is not a fold construct", value)
		}
	})
}

// TestRecoverToEscapedSource covers renderHTML's panic guard directly: safeURL
// closed the only input known to panic, so driving it through renderHTML would
// never reach the recovery.
func TestRecoverToEscapedSource(t *testing.T) {
	t.Parallel()

	const src = `<script>alert(1)</script> & "quoted"`

	got := recoverToEscapedSource(src, func() string { panic("boom") })
	assert.Equal(t, xhtml.EscapeString(src), got)
	// The point of escaping rather than returning the source: no markup escapes.
	assert.NotContains(t, got, "<script>")

	assert.Equal(t, "rendered",
		recoverToEscapedSource(src, func() string { return "rendered" }))
}

// TestHTMLFromMarkdownSafelink pins the behavior change Safelink brought to the
// shared renderer, whose one non-notification caller is
// OIDCConfig.SignupsDisabledText (coderd/userauth.go).
func TestHTMLFromMarkdownSafelink(t *testing.T) {
	t.Parallel()

	// Unsafe schemes stopped linking here, not just in notifications.
	for _, md := range []string{"[a](javascript:alert(1))", "[a](data:text/html;base64,eA==)"} {
		assert.NotContains(t, HTMLFromMarkdown(md), "<a ", "unsafe scheme linked: %s", md)
	}

	// Fragment and bare relative destinations stopped linking too, a silent loss
	// rather than a security property. See renderHTML.
	for _, md := range []string{"[a](#anchor)", "[a](docs/x.md)"} {
		assert.NotContains(t, HTMLFromMarkdown(md), "<a ", "destination %q now links", md)
	}

	// What the signups-disabled text actually uses must keep working.
	for _, tc := range []struct{ md, want string }{
		{"see https://coder.com/docs", `<a href="https://coder.com/docs"`},
		{"[docs](https://coder.com/docs)", `<a href="https://coder.com/docs"`},
		{"contact [us](mailto:support@coder.com)", `<a href="mailto:support@coder.com"`},
		{"**bold** and _italic_", "<strong>bold</strong>"},
	} {
		assert.Contains(t, HTMLFromMarkdown(tc.md), tc.want)
	}
}

// TestEscapeMarkdownResiduals pins the one gap EscapeMarkdown cannot close, so
// that closing it later is deliberate rather than accidental.
func TestEscapeMarkdownResiduals(t *testing.T) {
	t.Parallel()

	t.Run("CodeSpanSwallowsEscapes", func(t *testing.T) {
		t.Parallel()

		// CommonMark does not process escapes inside a code span, and the
		// workspace out-of-disk body wraps a value in one. The escaper cannot
		// see the sink, so it cannot choose not to escape.
		html := HTMLFromNotificationMarkdown("The volume `" + EscapeMarkdown("config[0]") + "` is full.")
		require.Contains(t, html, `config\[0\]`,
			"if this no longer leaks, the residual is closed and this test should become an assertion that it stays closed")
	})
}

// TestEscapeMarkdownNoStrayBackslash asserts the escaper's own invariant across
// every interpolation position a shipped template provides.
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
