package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// asciiPunctuation is every ASCII punctuation character, the set CommonMark
// declares escapable.
const asciiPunctuation = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

// TestEscapableSet pins which characters each renderer actually honors as a
// backslash escape. EscapeMarkdown's character classes are derived from this:
// escaping a character a renderer does not honor leaves a literal backslash in
// the rendered output, and both renderers see the same escaped string.
//
// Neither library honors all of CommonMark's escapable set, and they disagree
// with each other, so a dependency bump that shifts either table must fail here
// rather than silently ship backslashes into notification emails.
func TestEscapableSet(t *testing.T) {
	t.Parallel()

	// Measured against gomarkdown and glamour as vendored today.
	const (
		wantHTML  = "!#$&()*+-.:<>[\\]^_`{|}~"
		wantPlain = "!#()*+-.<>[\\]_`{|}"
	)

	var gotHTML, gotPlain strings.Builder
	for _, r := range asciiPunctuation {
		escaped := `X\` + string(r) + `Y`

		// HTML output escapes the markup characters, so compare against the
		// entity form where one applies.
		wantLiteral := "X" + string(r) + "Y"
		switch r {
		case '<':
			wantLiteral = "X&lt;Y"
		case '>':
			wantLiteral = "X&gt;Y"
		case '&':
			wantLiteral = "X&amp;Y"
		case '"':
			wantLiteral = "X&quot;Y"
		}
		html := strings.TrimSuffix(strings.TrimPrefix(HTMLFromMarkdown(escaped), "<p>"), "</p>")
		if html == wantLiteral {
			_, _ = gotHTML.WriteRune(r)
		}

		plain, err := PlaintextFromMarkdown(escaped)
		require.NoError(t, err)
		if plain == "X"+string(r)+"Y" {
			_, _ = gotPlain.WriteRune(r)
		}
	}

	require.Equal(t, wantHTML, gotHTML.String(),
		"the set of characters gomarkdown honors as escapes has changed; re-derive EscapeMarkdown's character classes")
	require.Equal(t, wantPlain, gotPlain.String(),
		"the set of characters glamour honors as escapes has changed; re-derive EscapeMarkdown's character classes")

	// Every character EscapeMarkdown emits a backslash before must be honored
	// by both renderers, or the escape shows up as literal text.
	for _, r := range inlineCritical + blockStart + leadingEmphasis {
		assert.Contains(t, wantHTML, string(r), "gomarkdown does not honor \\%s", string(r))
		assert.Contains(t, wantPlain, string(r), "glamour does not honor \\%s", string(r))
	}
	// Conversely, foldStart characters are handled by folding precisely because
	// they are not escapable.
	for _, r := range foldStart {
		assert.NotContains(t, wantPlain, string(r),
			"glamour now honors \\%s, so it could be escaped instead of folded", string(r))
	}
}

// TestEscapeMarkdownControlValues guards the label values that body templates
// compare with `eq`. Escaping one of these silently changes template control
// flow: it removed a whole paragraph from the AI budget notifications when "_"
// was escaped, with no error and no log line.
func TestEscapeMarkdownControlValues(t *testing.T) {
	t.Parallel()

	// Values compared against literals in shipped notification templates, plus
	// nearby shapes that must survive for the same reason.
	for _, v := range []string{
		"user_override", // migrations/000553, {{if eq .Labels.limit_source "user_override"}}
		"service",       // migrations/000568, {{if eq .Labels.account_type "service"}}
		"0",             // migrations/000480, {{if eq .Data.retention_days "0"}}
		"autobuild",
		"initiator",
		"user-override",
		"1.5",
		// A "." only closes a list marker when a space or the line end follows
		// it, so digit-dot values are untouched.
		"10.0.0.1",
		"bobby-workspace",
	} {
		require.Equal(t, v, EscapeMarkdown(v), "escaping changed a control value")
	}
}

func TestEscapeMarkdown(t *testing.T) {
	t.Parallel()

	// Tags that mean an untrusted value produced document structure. Emphasis
	// (<em>, <strong>, <del>) and code (<code>, <pre>) are accepted residuals:
	// they cannot carry a destination.
	structuralTags := []string{
		"<a ", "<img ", "<h1", "<h2", "<h3", "<h4", "<h5", "<h6",
		"<ul", "<ol", "<hr", "<blockquote", "<table",
	}

	// body mirrors the shape of the live TemplateUserAccountSuspended body: the
	// untrusted value sits mid-paragraph with trusted text on both sides.
	body := func(label string) string {
		return "The account belongs to **" + label + "** and it was suspended by **rob**."
	}

	t.Run("NeutralisesStructure", func(t *testing.T) {
		t.Parallel()

		// A row guards something only if its value produces structure without
		// escaping, so inertRaw has to be set deliberately. Leaving it unset on
		// a value that cannot produce structure fails the liveness assertion
		// below rather than passing as a test that asserts nothing.
		type structureCase struct {
			name  string
			value string
			// inertRaw marks a value that produces no structural tag even
			// unescaped. Each one is neutralized by something other than marker
			// escaping, and is covered non-vacuously by another test.
			inertRaw bool
		}

		for _, tc := range []structureCase{
			{name: "DisclosurePayload", value: "Eve\n## URGENT: SSO certificate expiring\n[Re-authenticate now](https://coder-sso.attacker.example/login)"},
			{name: "InlineLink", value: "[Re-authenticate now](https://attacker.example/login)"},
			// A link reference definition is not recognized mid-paragraph, so
			// this shape cannot form an anchor in the body helper's position.
			{name: "ReferenceLink", value: "[Re-auth][1]\n\n[1]: https://attacker.example", inertRaw: true},
			{name: "Image", value: "![px](https://tracker.attacker.example/p.gif)"},
			{name: "AngleAutolink", value: "Eve <https://attacker.example>"},
			// The notification renderer has autolinking disabled, which is what
			// neutralizes these two. See TestEscapeMarkdownNoAutolink.
			{name: "BareURL", value: "Eve https://attacker.example/login", inertRaw: true},
			{name: "Mailto", value: "Eve mailto:eve@attacker.example", inertRaw: true},
			{name: "ATXHeading", value: "Eve\n## URGENT"},
			{name: "SetextH1", value: "URGENT: re-auth required\n===\nx"},
			{name: "SetextH1Spaced", value: "Eve\n=== \n#### x"},
			{name: "SetextH1Repeated", value: "Eve\n===\n===\nx"},
			{name: "SetextH2", value: "Eve\n---\nx"},
			{name: "ThematicBreak", value: "Eve\n----\nx"},
			{name: "ThematicBreakStars", value: "Eve\n***\nnext"},
			{name: "ThematicBreakUnderscores", value: "Eve\n___\nnext"},
			{name: "ThematicBreakSpacedStars", value: "Eve\n* * *\nnext"},
			{name: "ThematicBreakSpacedUnderscores", value: "Eve\n_ _ _\nnext"},
			{name: "BulletList", value: "Eve\n- one\n- two"},
			{name: "BulletListStar", value: "Eve\n* one\n* two"},
			{name: "BulletListStarIndented", value: "Eve\n  * one\n  * two"},
			{name: "OrderedList", value: "Eve\n1. one\n2. two"},
			{name: "OrderedListMultiDigit", value: "Eve\n99. one\n100. two"},
			{name: "OrderedListParen", value: "Eve\n1) one\n2) two"},
			{name: "Blockquote", value: "Eve\n> quoted"},
			// The notification renderer no longer enables the Tables
			// extension, so no delimiter row forms a table here whatever the
			// escaper does. The escaper's own handling of both delimiter-row
			// spellings is covered non-vacuously by TestEscapeMarkdownColon,
			// which renders under CommonExtensions.
			{name: "Table", value: "a | b\n--- | ---\nc | d", inertRaw: true},
			// The safelink policy rejects the scheme, so no anchor forms even
			// unescaped. See TestEscapeMarkdownNoAutolink/SafelinkRejectsUnsafeSchemes.
			{name: "JavascriptScheme", value: "[click](javascript:alert(1))", inertRaw: true},
			// A value that already contains backslashes cannot be used to forge
			// an escape and re-enable link syntax. Inert by construction: the
			// value's own backslashes neutralize it, and the assertion is that
			// escaping does not re-enable it.
			{name: "EscapeForging", value: `Eve \[Re-auth\](https://attacker.example)`, inertRaw: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// Every value is rendered twice: as written, and with its line
				// breaks doubled. A blank line is what lets a block construct
				// interrupt the surrounding paragraph, so a marker can look
				// neutralized with a single break and still open a list or a
				// thematic break once a blank line precedes it.
				rawProducedTag := false
				for _, value := range []string{tc.value, strings.ReplaceAll(tc.value, "\n", "\n\n")} {
					escaped := EscapeMarkdown(value)
					html := HTMLFromNotificationMarkdown(body(escaped))
					plain, err := PlaintextFromMarkdown(body(escaped))
					require.NoError(t, err)
					// The same value with escaping removed, which is what shows
					// whether the assertions below depend on EscapeMarkdown.
					raw := HTMLFromNotificationMarkdown(body(value))

					for _, tag := range structuralTags {
						assert.NotContains(t, html, tag, "value %q rendered HTML: %s", value, html)
						rawProducedTag = rawProducedTag || strings.Contains(raw, tag)
					}
					// A backslash in the output is only acceptable if the value
					// contained one: otherwise a character was escaped that the
					// renderer does not honor, and the escape shows up as text.
					if !strings.Contains(value, `\`) {
						assert.NotContains(t, html, `\`, "value %q leaked a literal backslash into HTML", value)
						assert.NotContains(t, plain, `\`, "value %q leaked a literal backslash into plaintext", value)
					}
				}

				// Without this, a row whose value can never reach a line-start
				// position passes whether or not EscapeMarkdown runs. That is
				// how "Eve\n1. one" sat in this table asserting nothing: a list
				// cannot interrupt a paragraph without a blank line, so the
				// digit-dot was never in a position to open one.
				if !tc.inertRaw {
					assert.True(t, rawProducedTag,
						"vacuous row: %q produces no structural tag even unescaped, so the assertions above guard nothing; fix the value or set inertRaw with a reason",
						tc.value)
				}
			})
		}
	})

	t.Run("PreservesBenignValues", func(t *testing.T) {
		t.Parallel()

		// Escaping must be invisible for values that contain no structure. These
		// render byte-identically to the unescaped value, which is also why this
		// change leaves the notification golden files untouched.
		for _, value := range []string{
			"William Tables",
			"bobby-workspace",
			"Bobby's Template",
			"O'Brien-Smith (Eng) 100%",
			"autodeleted due to dormancy (autobuild)",
			"José Müller 日本語",
			// Documented multi-line custom notification, see
			// docs/admin/monitoring/notifications/index.md.
			"Test results:\n  • ✅ success",
			"Test results:\n  • ❌ failed (3 tests failed)",
		} {
			t.Run(value, func(t *testing.T) {
				t.Parallel()

				escaped := EscapeMarkdown(value)
				require.Equal(t, HTMLFromNotificationMarkdown(body(value)), HTMLFromNotificationMarkdown(body(escaped)))

				wantPlain, err := PlaintextFromMarkdown(body(value))
				require.NoError(t, err)
				gotPlain, err := PlaintextFromMarkdown(body(escaped))
				require.NoError(t, err)
				require.Equal(t, wantPlain, gotPlain)
			})
		}
	})

	t.Run("ControlCharacters", func(t *testing.T) {
		t.Parallel()

		for _, tc := range []struct {
			name  string
			value string
			want  string
		}{
			{"KeepsNewlines", "a\nb", "a\nb"},
			{"FoldsCarriageReturn", "a\r\nb", "a \nb"},
			{"FoldsTab", "a\tb", "a b"},
			{"FoldsVerticalTab", "a\vb", "a b"},
			{"DropsNul", "a\x00b", "ab"},
			{"DropsBell", "a\x07b", "ab"},
			{"DropsDelete", "a\x7fb", "ab"},
			{"Empty", "", ""},
		} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				require.Equal(t, tc.want, EscapeMarkdown(tc.value))
			})
		}
	})

	t.Run("AngleBracketsAreNeutralised", func(t *testing.T) {
		t.Parallel()

		// An angle-bracketed email address or URL is a CommonMark autolink, so a
		// display name of "Ops <ops@example.com>" currently renders as a mailto
		// anchor. Escaping "<" turns it into text, which is a deliberate
		// behavior change: the value is untrusted and must not carry a
		// destination.
		html := HTMLFromNotificationMarkdown(body(EscapeMarkdown("Ops <ops@example.com>")))
		require.NotContains(t, html, "<a ")
		require.Contains(t, html, "&lt;ops@example.com&gt;")
	})

	t.Run("EmphasisIsNotEscaped", func(t *testing.T) {
		t.Parallel()

		// Away from a line's leading position, "*" and "_" are left alone on
		// purpose: they cannot carry a destination, and escaping "_" corrupts
		// control values. Documenting the residual here so a future tightening
		// is a deliberate choice.
		//
		// Backtick is not in that group. It reaches a fenced code block, whose
		// info string gomarkdown writes into class="language-..." unescaped, so
		// it can carry a destination after all. See TestEscapeMarkdownFenceInfo.
		require.Equal(t, "Eve *_\\`~", EscapeMarkdown("Eve *_`~"))

		// In leading position "*" and "_" open a bullet list or a thematic
		// break, so the first one is escaped. "~" stays as it is: the fold in
		// EscapeMarkdown denies it the line-start position instead, because
		// glamour does not honor "\~".
		require.Equal(t, "\\*_\\`~", EscapeMarkdown("*_`~"))
		require.Equal(t, "\\_*\\`~", EscapeMarkdown("_*`~"))
	})
}

// TestEscapeMarkdownNoAutolink covers the R6 half of the fix: the notification
// renderer must not turn a URL in an untrusted value into an anchor, while
// explicit links in the trusted template markdown keep working.
func TestEscapeMarkdownNoAutolink(t *testing.T) {
	t.Parallel()

	t.Run("UntrustedValueProducesNoAnchor", func(t *testing.T) {
		t.Parallel()

		for _, value := range []string{
			"Eve https://attacker.example/login",
			"Eve [Re-auth](https://attacker.example/login)",
			"Eve <https://attacker.example>",
			"Eve mailto:eve@attacker.example",
			"Eve http://attacker.example",
		} {
			html := HTMLFromNotificationMarkdown("Account **" + EscapeMarkdown(value) + "** suspended.")
			assert.NotContains(t, html, "<a ", "value %q produced an anchor", value)
		}
	})

	t.Run("TrustedTemplateLinksStillRender", func(t *testing.T) {
		t.Parallel()

		// Shapes taken from shipped notification body templates.
		for _, markdown := range []string{
			"marked as [**dormant**](https://coder.com/docs/templates/schedule#dormancy-threshold-enterprise) because of x",
			"See [the docs](https://coder.com/docs/admin/templates/troubleshooting).",
		} {
			html := HTMLFromNotificationMarkdown(markdown)
			assert.Contains(t, html, `<a href="https://coder.com/docs/`, "trusted link did not render: %s", html)
		}
	})

	t.Run("HTMLFromMarkdownStillAutolinks", func(t *testing.T) {
		t.Parallel()

		// The shared renderer is unchanged, so the OIDC signups-disabled page
		// keeps its existing behavior.
		require.Contains(t, HTMLFromMarkdown("see https://coder.com/docs"), `<a href="https://coder.com/docs"`)
	})

	t.Run("SafelinkRejectsUnsafeSchemes", func(t *testing.T) {
		t.Parallel()

		for _, dest := range []string{"javascript:alert(1)", "data:text/html;base64,PHNjcmlwdD4="} {
			html := HTMLFromNotificationMarkdown(fmt.Sprintf("[click](%s)", dest))
			assert.NotContains(t, html, "<a ", "unsafe scheme %q was linked", dest)
		}
	})
}
