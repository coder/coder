package dispatch

import (
	"html"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
	markdown "github.com/coder/coder/v2/coderd/render"
)

// appearanceHelpers returns the HTML template's helpers with benign deployment
// values, so a test asserting on injected markup measures only what the
// untrusted payload contributed.
func appearanceHelpers() map[string]any {
	return map[string]any{
		"base_url":     func() string { return "https://coder.example.com" },
		"current_year": func() string { return "2026" },
		"logo_url":     func() string { return "https://coder.example.com/logo.png" },
		"app_name":     func() string { return "Coder" },
	}
}

// TestSMTPHTMLTemplateEscapesSubjectAndUserName covers the sinks that Markdown
// escaping cannot reach. The subject is produced by PlaintextFromMarkdown, which
// strips Markdown and decodes HTML entities, so an entity-encoded payload in a
// label arrives at html.gotmpl as raw HTML. "&" is not backslash-escapable in
// either renderer, so EscapeMarkdown cannot stop it and the fix has to be at the
// template sink. UserName reaches the same template straight from the unescaped
// payload the dispatcher is handed.
func TestSMTPHTMLTemplateEscapesSubjectAndUserName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		// title is the rendered title template handed to the dispatcher, as
		// notifier.prepare produces it.
		title string
		// userName is the recipient's own display name.
		userName string
		// injected is the markup the untrusted value resolves to by the time it
		// reaches the template. Asserting on the template's own tags would not
		// work: html.gotmpl legitimately renders anchors in its footer.
		injected string
	}{
		{
			name: "EntityEncodedAnchorInSubject",
			// Shape of a template_display_name interpolated into the shipped
			// title template. PlaintextFromMarkdown decodes the entities.
			title:    `Template "&lt;a href="https://attacker.example/login"&gt;Re-authenticate now&lt;/a&gt;" deleted`,
			userName: "Bobby",
			injected: `<a href="https://attacker.example/login">Re-authenticate now</a>`,
		},
		{
			name:     "EntityEncodedImageInSubject",
			title:    `Workspace "&lt;img src=x onerror="alert(1)"&gt;" marked dormant`,
			userName: "Bobby",
			injected: `<img src=x onerror="alert(1)">`,
		},
		{
			name:     "RawHTMLInUserName",
			title:    "Account suspended",
			userName: `Bobby <img src=x onerror="alert(1)">`,
			injected: `<img src=x onerror="alert(1)">`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// The two steps SMTPHandler.Dispatcher performs before the HTML
			// template is rendered.
			subject, err := markdown.PlaintextFromMarkdown(tc.title)
			require.NoError(t, err)

			payload := types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               tc.userName,
				Labels: map[string]string{
					"_subject": subject,
					"_body":    "<p>Test body</p>",
				},
			}

			got, err := render.GoTemplate(htmlTemplate, payload, appearanceHelpers())
			require.NoError(t, err)

			// The case has to carry markup, or the assertions below hold
			// whether or not the template escapes anything.
			escaped := html.EscapeString(tc.injected)
			require.NotEqual(t, tc.injected, escaped,
				"case carries no HTML to escape, so it guards nothing")

			require.NotContains(t, got, tc.injected,
				"untrusted markup reached the rendered email: %s", got)
			require.Contains(t, got, escaped,
				"the value must still be displayed, entity encoded: %s", got)
		})
	}
}

func TestSMTPHTMLTemplateEscapesAppearanceHelpers(t *testing.T) {
	t.Parallel()

	const (
		appName = `Coder"><script>alert(1)</script>`
		logoURL = `https://example.com/logo.png"><img src=x onerror=alert(1)>`
	)

	payload := types.MessagePayload{
		NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
		UserName:               "Test User",
		Labels: map[string]string{
			"_subject": "Test notification",
			"_body":    "<p>Test body</p>",
		},
	}
	helpers := map[string]any{
		"base_url":     func() string { return "https://coder.example.com" },
		"current_year": func() string { return "2026" },
		"logo_url":     func() string { return logoURL },
		"app_name":     func() string { return appName },
	}

	got, err := render.GoTemplate(htmlTemplate, payload, helpers)
	require.NoError(t, err)

	require.True(t, strings.Contains(got, html.EscapeString(appName)), "application name must be HTML escaped")
	require.True(t, strings.Contains(got, html.EscapeString(logoURL)), "logo URL must be HTML escaped")
	require.False(t, strings.Contains(got, appName), "raw application name must not be rendered")
	require.False(t, strings.Contains(got, logoURL), "raw logo URL must not be rendered")
}

func TestValidateFromAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		input              string
		expectedEnvelope   string
		expectedHeader     string
		expectedErrContain string
	}{
		{
			name:             "bare email address",
			input:            "system@coder.com",
			expectedEnvelope: "system@coder.com",
			expectedHeader:   "system@coder.com",
		},
		{
			name:             "email with display name",
			input:            "Coder System <system@coder.com>",
			expectedEnvelope: "system@coder.com",
			expectedHeader:   "Coder System <system@coder.com>",
		},
		{
			name:             "email with quoted display name",
			input:            `"Coder Notifications" <notifications@coder.com>`,
			expectedEnvelope: "notifications@coder.com",
			expectedHeader:   `"Coder Notifications" <notifications@coder.com>`,
		},
		{
			name:             "email with special characters in display name",
			input:            `"O'Brien, John" <john@example.com>`,
			expectedEnvelope: "john@example.com",
			expectedHeader:   `"O'Brien, John" <john@example.com>`,
		},
		{
			name:               "invalid email address",
			input:              "not-an-email",
			expectedErrContain: "parse 'from' address",
		},
		{
			name:               "empty string",
			input:              "",
			expectedErrContain: "parse 'from' address",
		},
		{
			name:               "multiple addresses",
			input:              "a@example.com, b@example.com",
			expectedErrContain: "'from' address not defined",
		},
	}

	handler := &SMTPHandler{}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			envelope, header, err := handler.validateFromAddr(tc.input)

			if tc.expectedErrContain != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.expectedErrContain)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedEnvelope, envelope,
				"envelope address should be the bare email")
			require.Equal(t, tc.expectedHeader, header,
				"header address should preserve the original input")
		})
	}
}

func TestEncodeHeaderValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "ascii is unchanged",
			value: `User account "bobby" suspended`,
			want:  `User account "bobby" suspended`,
		},
		{
			name:  "crlf is folded",
			value: "Subject\r\nBcc: attacker@example.com",
			want:  "Subject  Bcc: attacker@example.com",
		},
		{
			name:  "bare newline is folded",
			value: "Subject\nBcc: attacker@example.com",
			want:  "Subject Bcc: attacker@example.com",
		},
		{
			name:  "non-ascii is q-encoded",
			value: "Konto gelöscht",
			want:  "=?utf-8?q?Konto_gel=C3=B6scht?=",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := encodeHeaderValue(tc.value)
			require.Equal(t, tc.want, got)
			// Whatever the input, the result must never be able to terminate the
			// header it is written into.
			require.NotContains(t, got, "\r")
			require.NotContains(t, got, "\n")
		})
	}
}
