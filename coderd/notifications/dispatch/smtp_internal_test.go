package dispatch

import (
	"html"
	"mime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
	markdown "github.com/coder/coder/v2/coderd/render"
)

// Benign values, so a test measures only what its own payload injected.
func templateHelpers() map[string]any {
	return map[string]any{
		"base_url":     func() string { return "https://coder.example.com" },
		"current_year": func() string { return "2026" },
		"logo_url":     func() string { return "https://coder.example.com/logo.png" },
		"app_name":     func() string { return "Coder" },
	}
}

func TestSMTPHTMLTemplateEscapesUntrustedValues(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		title    string
		userName string
		actions  []types.TemplateAction
		injected string
	}{
		{
			name:     "EntityEncodedAnchorInSubject",
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
		{
			name:     "RawHTMLInActionLabel",
			title:    "Account suspended",
			userName: "Bobby",
			actions: []types.TemplateAction{
				{Label: `<img src=x onerror="alert(1)">`, URL: "https://coder.example.com/"},
			},
			injected: `<img src=x onerror="alert(1)">`,
		},
		{
			name:     "RawHTMLInActionURL",
			title:    "Account suspended",
			userName: "Bobby",
			actions: []types.TemplateAction{
				{Label: "Open Coder", URL: `https://coder.example.com/?x=<script>alert(1)</script>`},
			},
			injected: `<script>alert(1)</script>`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Decodes the entities, so the title arrives as live markup.
			subject, err := markdown.PlaintextFromMarkdown(tc.title)
			require.NoError(t, err)

			// Actions are set as the template sees them. The enqueuer renders
			// them into JSON first, which rejects a `"` of its own accord.
			payload := types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               tc.userName,
				Actions:                tc.actions,
				Labels: map[string]string{
					"_subject": subject,
					"_body":    "<p>Test body</p>",
				},
			}

			got, err := render.GoTemplate(htmlTemplate, payload, templateHelpers())
			require.NoError(t, err)

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
	helpers := templateHelpers()
	helpers["logo_url"] = func() string { return logoURL }
	helpers["app_name"] = func() string { return appName }

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
			// The result must never be able to terminate its own header.
			require.NotContains(t, got, "\r")
			require.NotContains(t, got, "\n")
		})
	}
}

// TestEncodeHeaderValueEncodedWord covers a forged RFC 2047 encoded-word, which
// is printable ASCII and so passes mime.WordEncoder through to the client.
func TestEncodeHeaderValueEncodedWord(t *testing.T) {
	t.Parallel()

	// Decodes to "URGENT: verify your account".
	const forged = "=?utf-8?B?VVJHRU5UOiB2ZXJpZnkgeW91ciBhY2NvdW50?="
	got := encodeHeaderValue(forged + " shared a chat with you")

	// The forged word must not survive as something a client would decode.
	require.NotContains(t, got, forged)

	// Decoded rather than compared: chunk boundaries are an implementation detail.
	decoded, err := new(mime.WordDecoder).DecodeHeader(got)
	require.NoError(t, err)
	require.Equal(t, forged+" shared a chat with you", decoded)
}

// TestEncodeHeaderValueFolds covers RFC 5322's 998-octet line limit, which
// mime.WordEncoder does not fold for.
func TestEncodeHeaderValueFolds(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]string{
		"non-ascii": strings.Repeat("é", 600),
		"ascii":     strings.Repeat("a b ", 400),
		// A rune that does not divide evenly into the per-word budget must not
		// be split across two encoded-words: each has to decode on its own.
		"multibyte": strings.Repeat("日本語", 400),
		// Under the raw byte limit and over it once Q-encoded, so these fail
		// unless the gate measures the encoded form.
		"200 accented runes": strings.Repeat("é", 200),
		"300 cjk runes":      strings.Repeat("日", 300),
		"200 emoji":          strings.Repeat("🎉", 200),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := encodeHeaderValue(value)
			for _, line := range strings.Split(got, "\r\n") {
				require.LessOrEqual(t, len(line), 998,
					"a header line exceeds RFC 5322's limit: %d octets", len(line))
			}
			// A CRLF must begin a continuation, or this is injection not folding.
			for _, after := range strings.Split(got, "\r\n")[1:] {
				require.True(t, strings.HasPrefix(after, " "),
					"a CRLF was not followed by folding whitespace: %q", got)
			}

			decoded, err := new(mime.WordDecoder).DecodeHeader(got)
			require.NoError(t, err)
			require.Equal(t, value, decoded)
		})
	}
}
