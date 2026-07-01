package dispatch

import (
	"html"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
)

func TestSMTPHTMLTemplateEscapesUntrustedValues(t *testing.T) {
	t.Parallel()

	const (
		appName  = `Coder"><script>alert(1)</script>`
		logoURL  = `https://example.com/logo.png"><img src=x onerror=alert(1)>`
		userName = `Eve<img src=x onerror=alert(1)>`
		// _subject is produced by PlaintextFromMarkdown, which decodes HTML
		// entities, so it can contain literal HTML if a title interpolates an
		// untrusted value. It must be HTML-escaped where it lands in markup.
		subject = `Alert<img src=x onerror=alert(1)>`
	)

	payload := types.MessagePayload{
		NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
		UserName:               userName,
		Labels: map[string]string{
			"_subject": subject,
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
	require.True(t, strings.Contains(got, html.EscapeString(userName)), "recipient name must be HTML escaped")
	require.True(t, strings.Contains(got, html.EscapeString(subject)), "subject must be HTML escaped")
	require.False(t, strings.Contains(got, appName), "raw application name must not be rendered")
	require.False(t, strings.Contains(got, logoURL), "raw logo URL must not be rendered")
	require.False(t, strings.Contains(got, userName), "raw recipient name must not be rendered")
	require.False(t, strings.Contains(got, subject), "raw subject must not be rendered")
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
