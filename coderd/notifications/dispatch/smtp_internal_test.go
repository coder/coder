package dispatch

import (
	"html"
	"mime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/notifications/render"
	"github.com/coder/coder/v2/coderd/notifications/types"
)

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

// TestEncodeHeaderValueEncodedWord covers a value that already looks like an
// RFC 2047 encoded-word. mime.WordEncoder only encodes non-ASCII, and an
// encoded-word is pure printable ASCII, so a forged one would reach the
// recipient's client intact and be decoded there.
func TestEncodeHeaderValueEncodedWord(t *testing.T) {
	t.Parallel()

	// Decodes to "URGENT: verify your account".
	const forged = "=?utf-8?B?VVJHRU5UOiB2ZXJpZnkgeW91ciBhY2NvdW50?="
	got := encodeHeaderValue(forged + " shared a chat with you")

	// The forged word must not survive as something a client would decode.
	require.NotContains(t, got, forged)

	// And the real text must round-trip, so the fix costs no fidelity. A
	// decoder is the right assertion here: the exact chunk boundaries are an
	// implementation detail, but what the recipient sees is not.
	decoded, err := new(mime.WordDecoder).DecodeHeader(got)
	require.NoError(t, err)
	require.Equal(t, forged+" shared a chat with you", decoded)
}

// TestEncodeHeaderValueFolds covers RFC 5322's 998-octet line limit.
// mime.WordEncoder separates its encoded-words with a space, so a long value
// stays on one line however far past the limit it runs.
func TestEncodeHeaderValueFolds(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]string{
		"non-ascii": strings.Repeat("é", 600),
		"ascii":     strings.Repeat("a b ", 400),
		// A rune that does not divide evenly into the per-word budget must not
		// be split across two encoded-words: each has to decode on its own.
		"multibyte": strings.Repeat("日本語", 400),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := encodeHeaderValue(value)
			for _, line := range strings.Split(got, "\r\n") {
				require.LessOrEqual(t, len(line), 998,
					"a header line exceeds RFC 5322's limit: %d octets", len(line))
			}
			// Every CRLF must begin a folded continuation rather than end the
			// header, or this is header injection instead of folding.
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
