package render_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
	coderrender "github.com/coder/coder/v2/coderd/render"
)

func TestSanitizeMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		contains []string // substrings the output must contain
		absent   []string // substrings the output must NOT contain
		exact    string   // exact expected output, empty to skip
	}{
		{
			name:  "plain text unchanged",
			input: "Hello World",
			exact: "Hello World",
		},
		{
			name:  "empty string",
			input: "",
			exact: "",
		},
		{
			name:  "unicode names preserved",
			input: "José García",
			exact: "José García",
		},
		{
			name:  "CJK names preserved",
			input: "田中太郎",
			exact: "田中太郎",
		},
		{
			name:     "markdown link escaped",
			input:    "[Click me](https://evil.example)",
			absent:   []string{"[Click me]"},
			contains: []string{"\\[", "\\]", "\\(", "\\)"},
		},
		{
			name:     "heading injection escaped",
			input:    "## URGENT heading",
			absent:   []string{"## "},
			contains: []string{"\\#\\# URGENT heading"},
		},
		{
			name:     "image injection escaped",
			input:    "![tracker](https://evil.example/pixel.gif)",
			absent:   []string{"![tracker]("},
			contains: []string{"\\!", "\\[tracker\\]"},
		},
		{
			name:     "bold/italic escaped",
			input:    "**bold** and *italic*",
			contains: []string{"\\*\\*bold\\*\\*", "\\*italic\\*"},
		},
		{
			name:  "newlines collapsed to spaces",
			input: "line1\nline2\nline3",
			exact: "line1 line2 line3",
		},
		{
			name:  "carriage returns removed",
			input: "line1\r\nline2",
			exact: "line1 line2",
		},
		{
			name:     "backtick code escaped",
			input:    "`code block`",
			contains: []string{"\\`code block\\`"},
		},
		{
			name:     "pipe table escaped",
			input:    "| col1 | col2 |",
			contains: []string{"\\|"},
		},
		{
			name:     "blockquote escaped",
			input:    "> quoted text",
			contains: []string{"\\>"},
		},
		{
			name:     "underscore escaped",
			input:    "_emphasis_",
			contains: []string{"\\_emphasis\\_"},
		},
		{
			name:     "full phishing payload",
			input:    "Eve\n## URGENT: SSO certificate expiring\n[Re-authenticate now](https://coder-sso.attacker.example/login)",
			absent:   []string{"\n", "[Re-authenticate now]"},
			contains: []string{"Eve ", "\\#\\# URGENT"},
		},
		{
			name:     "javascript URI in markdown link",
			input:    "[Click](javascript:alert(1))",
			absent:   []string{"[Click](javascript"},
			contains: []string{"\\[Click\\]\\(javascript"},
		},
		{
			name:  "parentheses in normal name",
			input: "Jane (Admin)",
			exact: "Jane \\(Admin\\)",
		},
		{
			// Regression: angle-bracket autolink must not survive. Escaping ">"
			// alone was insufficient because "<" was never escaped, so the
			// "<...>" autolink still opened (SEC-93 / GHSA-2w2x-w3c8-9jrw
			// follow-up). The escaped output still contains the "<" byte (as
			// "\<"), so correctness is asserted via the exact escaped form here;
			// the "no clickable <a href>" guarantee is covered end-to-end in
			// TestSMTPHTMLTemplateMarkdownInjection.
			name:  "angle-bracket autolink escaped",
			input: "<https://evil.example/login>",
			exact: "\\<https://evil.example/login\\>",
		},
		{
			// Regression: the backslash-tweaked bypass. Without escaping "\",
			// a user backslash combined with the sanitizer's own "\>" to leave
			// a live autolink terminator.
			name:  "backslash bypass of autolink escaped",
			input: "<https://evil.example/login\\>",
			exact: "\\<https://evil.example/login\\\\\\>",
		},
		{
			name:  "lone backslash escaped",
			input: "back\\slash",
			exact: "back\\\\slash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := render.SanitizeMarkdown(tt.input)

			if tt.exact != "" {
				assert.Equal(t, tt.exact, result)
			}
			for _, s := range tt.contains {
				assert.Contains(t, result, s, "output should contain %q", s)
			}
			for _, s := range tt.absent {
				assert.NotContains(t, result, s, "output should NOT contain %q", s)
			}
		})
	}
}

func TestSanitizedPayload(t *testing.T) {
	t.Parallel()

	t.Run("sanitizes UserName and Labels in the copy", func(t *testing.T) {
		t.Parallel()
		payload := types.MessagePayload{
			UserName:     "[evil](https://evil.example)",
			UserEmail:    "user@example.com",
			UserUsername: "normaluser",
			Labels: map[string]string{
				"initiator":                 "admin",
				"created_account_user_name": "## Heading\n[link](https://evil.example)",
				"safe_label":                "no-special-chars",
			},
		}
		sanitized := render.SanitizedPayload(payload)

		// UserName should be sanitized in the copy
		assert.NotContains(t, sanitized.UserName, "[evil]")
		assert.Contains(t, sanitized.UserName, "\\[evil\\]")

		// Labels should be sanitized in the copy
		assert.NotContains(t, sanitized.Labels["created_account_user_name"], "## Heading")
		assert.Contains(t, sanitized.Labels["created_account_user_name"], "\\#\\# Heading")
		assert.NotContains(t, sanitized.Labels["created_account_user_name"], "\n")

		// Non-special labels unchanged
		assert.Equal(t, "no-special-chars", sanitized.Labels["safe_label"])
		assert.Equal(t, "admin", sanitized.Labels["initiator"])

		// Fields NOT in scope are NOT modified
		assert.Equal(t, "user@example.com", sanitized.UserEmail)
		assert.Equal(t, "normaluser", sanitized.UserUsername)
	})

	t.Run("leaves skipped control-flow labels verbatim", func(t *testing.T) {
		t.Parallel()
		// Labels named in skipLabels hold machine-set enum values used in Go
		// template control flow. Escaping them (e.g. "user_override" ->
		// "user\_override") would break comparisons like
		// {{if eq .Labels.limit_source "user_override"}}, so they must be left
		// verbatim while every other user-controlled label is still escaped.
		payload := types.MessagePayload{
			UserName: "[evil](https://evil.example)",
			Labels: map[string]string{
				"limit_source": "user_override",
				"username":     "## not a heading_value",
			},
		}
		sanitized := render.SanitizedPayload(payload, "limit_source")

		// Skipped label is untouched, including the underscore.
		assert.Equal(t, "user_override", sanitized.Labels["limit_source"])

		// Other labels and UserName are still escaped.
		assert.Equal(t, "\\#\\# not a heading\\_value", sanitized.Labels["username"])
		assert.Contains(t, sanitized.UserName, "\\[evil\\]")
	})

	t.Run("leaves the original payload untouched", func(t *testing.T) {
		t.Parallel()
		// The original must keep its raw values so non-Markdown consumers
		// (webhook JSON, SMTP greeting, plaintext) receive them verbatim.
		payload := types.MessagePayload{
			UserName: "Jane (Admin)",
			Labels: map[string]string{
				"template_version": "angry_torvalds",
			},
		}
		sanitized := render.SanitizedPayload(payload)

		// Copy is escaped.
		assert.Equal(t, "Jane \\(Admin\\)", sanitized.UserName)
		assert.Equal(t, "angry\\_torvalds", sanitized.Labels["template_version"])

		// Original is verbatim.
		assert.Equal(t, "Jane (Admin)", payload.UserName)
		assert.Equal(t, "angry_torvalds", payload.Labels["template_version"])
	})

	t.Run("handles nil labels map", func(t *testing.T) {
		t.Parallel()
		payload := types.MessagePayload{
			UserName: "test",
			Labels:   nil,
		}
		var sanitized types.MessagePayload
		require.NotPanics(t, func() {
			sanitized = render.SanitizedPayload(payload)
		})
		assert.Nil(t, sanitized.Labels)
	})

	t.Run("handles empty payload", func(t *testing.T) {
		t.Parallel()
		var sanitized types.MessagePayload
		require.NotPanics(t, func() {
			sanitized = render.SanitizedPayload(types.MessagePayload{})
		})
		assert.Equal(t, "", sanitized.UserName)
	})
}

func TestSanitizeMarkdownInTemplateRendering(t *testing.T) {
	t.Parallel()

	// End-to-end: sanitize -> GoTemplate -> verify no injection in output
	t.Run("malicious display name in body template", func(t *testing.T) {
		t.Parallel()
		payload := types.MessagePayload{
			UserName: "Eve\n## URGENT\n[Click](https://evil.example)",
			Labels: map[string]string{
				"created_account_name":      "eviluser",
				"created_account_user_name": "Eve\n## URGENT\n[Click](https://evil.example)",
				"initiator":                 "admin",
			},
		}
		sanitized := render.SanitizedPayload(payload)

		bodyTmpl := `New user account **{{.Labels.created_account_name}}** has been created.

This new user account was created {{if .Labels.created_account_user_name}}for **{{.Labels.created_account_user_name}}** {{end}}by **{{.Labels.initiator}}**.`

		body, err := render.GoTemplate(bodyTmpl, sanitized, nil)
		require.NoError(t, err)

		// The rendered markdown should not contain unescaped link/heading syntax
		assert.NotContains(t, body, "[Click](")
		assert.NotContains(t, body, "## URGENT")
		// Should still contain escaped versions
		assert.Contains(t, body, "\\[Click\\]")
	})

	t.Run("control-flow label comparison survives sanitization", func(t *testing.T) {
		t.Parallel()
		// Regression: the AI budget admin templates gate a line on
		// {{if eq .Labels.limit_source "user_override"}}. Because the label
		// value contains an underscore, escaping it would break the eq
		// comparison and silently drop the line. Skipping the control-flow
		// label keeps the comparison working while user-controlled labels stay
		// escaped.
		payload := types.MessagePayload{
			Labels: map[string]string{
				"username":     "[evil](https://evil.example)",
				"limit_source": "user_override",
			},
		}
		sanitized := render.SanitizedPayload(payload, "limit_source")

		bodyTmpl := `User **{{.Labels.username}}** reached their limit.` + "\n\n" +
			`{{- if eq .Labels.limit_source "user_override"}}` + "\n\n" +
			`This limit is a per-user override.` + "\n" +
			`{{- end}}`

		body, err := render.GoTemplate(bodyTmpl, sanitized, nil)
		require.NoError(t, err)

		// The conditional line renders because limit_source was not escaped.
		assert.Contains(t, body, "This limit is a per-user override.")
		// The user-controlled label is still escaped.
		assert.NotContains(t, body, "[evil](")
		assert.Contains(t, body, "\\[evil\\]")
	})

	t.Run("bold formatting in template still works", func(t *testing.T) {
		t.Parallel()
		payload := types.MessagePayload{
			Labels: map[string]string{
				"name": "my-workspace",
			},
		}
		sanitized := render.SanitizedPayload(payload)

		body, err := render.GoTemplate(
			`Workspace **{{.Labels.name}}** has been deleted.`,
			sanitized, nil)
		require.NoError(t, err)

		// Template-level bold markers should survive (they're in the template, not labels)
		assert.Contains(t, body, "**my-workspace**")
	})

	t.Run("label with asterisks does not break bold", func(t *testing.T) {
		t.Parallel()
		payload := types.MessagePayload{
			Labels: map[string]string{
				"name": "workspace*test",
			},
		}
		sanitized := render.SanitizedPayload(payload)

		body, err := render.GoTemplate(
			`Workspace **{{.Labels.name}}** updated.`,
			sanitized, nil)
		require.NoError(t, err)

		// The escaped asterisk in the value should not interfere with template bold
		assert.Contains(t, body, `**workspace\*test**`)
	})
}

func TestHTMLFromMarkdownSafe(t *testing.T) {
	t.Parallel()

	// Import the render package for HTMLFromMarkdownSafe
	// We test it indirectly through the notification render package

	t.Run("sanitized payload produces no links in final HTML", func(t *testing.T) {
		t.Parallel()
		payload := types.MessagePayload{
			Labels: map[string]string{
				"name": "[evil](https://evil.example)",
			},
		}
		sanitized := render.SanitizedPayload(payload)

		body, err := render.GoTemplate(
			`Account **{{.Labels.name}}** created.`,
			sanitized, nil)
		require.NoError(t, err)

		// Render the sanitized body to HTML exactly as the SMTP dispatcher
		// does, and assert the attacker link never becomes an <a> tag. The URL
		// still appears as inert literal text, which is fine; what matters is
		// that it is not a clickable link.
		html := coderrender.HTMLFromMarkdownSafe(body)
		assert.NotContains(t, html, "<a", "sanitized body must not produce a link")
		assert.Contains(t, html, "<strong>", "legitimate template formatting should still render")
	})
}
