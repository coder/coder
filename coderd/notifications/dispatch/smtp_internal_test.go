package dispatch

import (
	"html"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	markdown "github.com/coder/coder/v2/coderd/render"

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

// TestSMTPHTMLTemplateMarkdownInjection is an end-to-end regression test that
// exercises the full notification email rendering pipeline with malicious input.
// It simulates the complete flow: SanitizePayload -> GoTemplate (body) ->
// HTMLFromMarkdownSafe -> GoTemplate (outer HTML template) and asserts that the
// final HTML email output contains no attacker-controlled links, headings, images,
// scripts, or other injected content.
//
// If this test fails, it likely means a change to the rendering pipeline has
// introduced a Markdown or HTML injection vulnerability in notification emails.
func TestSMTPHTMLTemplateMarkdownInjection(t *testing.T) {
	t.Parallel()

	helpers := map[string]any{
		"base_url":     func() string { return "https://coder.example.com" },
		"current_year": func() string { return "2026" },
		"logo_url":     func() string { return "https://coder.example.com/logo.png" },
		"app_name":     func() string { return "Coder" },
	}

	tests := []struct {
		name string
		// payload is the unsanitized notification payload (as it would arrive
		// from the database before SanitizePayload runs).
		payload types.MessagePayload
		// bodyTemplate is the notification body template (stored in the DB).
		bodyTemplate string
		// absentInHTML lists substrings that must NOT appear in the final HTML
		// email output.
		absentInHTML []string
		// presentInHTML lists substrings that MUST appear in the final HTML.
		presentInHTML []string
	}{
		{
			name: "markdown link injection via display name",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "Eve\n[Re-authenticate](https://evil.example/login)",
				Labels: map[string]string{
					"created_account_user_name": "Eve\n[Re-authenticate](https://evil.example/login)",
					"initiator":                 "admin",
				},
			},
			bodyTemplate: `Account created for **{{.Labels.created_account_user_name}}** by **{{.Labels.initiator}}**.`,
			absentInHTML: []string{
				`<a href="https://evil.example`,
				`href="https://evil.example`,
			},
			presentInHTML: []string{
				"<strong>admin</strong>",
			},
		},
		{
			name: "heading injection via display name",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "Eve\n## URGENT SECURITY ALERT",
				Labels: map[string]string{
					"suspended_account_user_name": "Eve\n## URGENT SECURITY ALERT",
					"initiator":                   "admin",
				},
			},
			bodyTemplate: `Account **{{.Labels.suspended_account_user_name}}** suspended by **{{.Labels.initiator}}**.`,
			absentInHTML: []string{
				"<h2>",
				"<h1>",
				"<h3>",
			},
			presentInHTML: []string{
				"## URGENT SECURITY ALERT", // rendered as literal text
			},
		},
		{
			name: "image tracking pixel injection",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "![](https://evil.example/track.gif)",
				Labels: map[string]string{
					"name": "![](https://evil.example/track.gif)",
				},
			},
			bodyTemplate: `Workspace **{{.Labels.name}}** deleted.`,
			absentInHTML: []string{
				`src="https://evil.example`, // no attacker-controlled image source
			},
			presentInHTML: []string{
				"![]", // rendered as literal text
			},
		},
		{
			name: "javascript URI via markdown link",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "normal",
				Labels: map[string]string{
					"name": "[xss](javascript:alert(document.cookie))",
				},
			},
			bodyTemplate: `Workspace **{{.Labels.name}}** updated.`,
			absentInHTML: []string{
				`href="javascript:`, // no javascript link
			},
		},
		{
			name: "raw HTML injection in display name",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               `<script>alert(1)</script>`,
				Labels:                 map[string]string{},
			},
			bodyTemplate: `Hello.`,
			absentInHTML: []string{
				"<script>", // raw script tag must not appear
			},
		},
		{
			name: "bare URL autolink in label value",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "admin",
				Labels: map[string]string{
					"name": "workspace https://evil.example test",
				},
			},
			bodyTemplate: `Workspace **{{.Labels.name}}** created.`,
			absentInHTML: []string{
				`<a href="https://evil.example"`,
			},
		},
		{
			// Regression: CommonMark angle-bracket autolink via display name.
			// Escaping ">" alone did not stop this ("<" was never escaped).
			name: "angle-bracket autolink via display name",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "<https://evil.example/login>",
				Labels: map[string]string{
					"created_account_user_name": "<https://evil.example/login>",
					"initiator":                 "admin",
				},
			},
			bodyTemplate: `Account created for **{{.Labels.created_account_user_name}}** by **{{.Labels.initiator}}**.`,
			absentInHTML: []string{
				`href="https://evil.example`,
			},
			presentInHTML: []string{
				"<strong>admin</strong>",
			},
		},
		{
			// Regression: the backslash-tweaked bypass of the angle-bracket
			// autolink escape. A user backslash combined with the sanitizer's
			// "\>" left a live autolink terminator until "\" was also escaped.
			name: "backslash-tweaked autolink bypass via display name",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "<https://evil.example/login\\>",
				Labels: map[string]string{
					"created_account_user_name": "<https://evil.example/login\\>",
					"initiator":                 "admin",
				},
			},
			bodyTemplate: `Account created for **{{.Labels.created_account_user_name}}** by **{{.Labels.initiator}}**.`,
			absentInHTML: []string{
				`href="https://evil.example`,
			},
			presentInHTML: []string{
				"<strong>admin</strong>",
			},
		},
		{
			name: "legitimate template content renders correctly",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "John Smith",
				Labels: map[string]string{
					"workspace": "my-dev-env",
					"initiator": "admin",
				},
			},
			bodyTemplate: `Workspace **{{.Labels.workspace}}** was updated by **{{.Labels.initiator}}**.`,
			presentInHTML: []string{
				"Hi John Smith,",
				"<strong>my-dev-env</strong>",
				"<strong>admin</strong>",
			},
		},
		{
			name: "unicode display name preserved",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "Jos\u00e9 Garc\u00eda",
				Labels:                 map[string]string{"name": "workspace-1"},
			},
			bodyTemplate: `Workspace **{{.Labels.name}}** ready.`,
			presentInHTML: []string{
				"Jos\u00e9 Garc\u00eda",
				"<strong>workspace-1</strong>",
			},
		},
		// Legitimate rendering regression tests below. These verify that
		// production notification features continue working after the
		// security hardening.
		{
			name: "template-authored markdown link renders correctly",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "user",
				Labels: map[string]string{
					"name":   "my-workspace",
					"reason": "inactivity",
				},
			},
			bodyTemplate: `Your workspace **{{.Labels.name}}** has been marked as [**dormant**](https://coder.com/docs/templates/schedule#dormancy-threshold-enterprise) because of {{.Labels.reason}}.`,
			presentInHTML: []string{
				`<a href="https://coder.com/docs/templates/schedule#dormancy-threshold-enterprise">`,
				"<strong>dormant</strong>",
				"<strong>my-workspace</strong>",
			},
		},
		{
			name: "markdown list renders correctly",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "admin",
				Labels:                 map[string]string{},
			},
			bodyTemplate: "Build failures:\n\n- **template-v1** failed 3 times\n- **template-v2** failed 1 time",
			presentInHTML: []string{
				"<li>",
				"<strong>template-v1</strong>",
				"<strong>template-v2</strong>",
			},
		},
		{
			name: "emphasis and code spans render correctly",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "admin",
				Labels: map[string]string{
					"workspace": "my-ws",
				},
			},
			bodyTemplate: "Volume **`/home/coder`** is over 90%% full in workspace **{{.Labels.workspace}}**.",
			presentInHTML: []string{
				"<code>/home/coder</code>",
				"<strong>my-ws</strong>",
			},
		},
		{
			name: "action buttons render with proper URLs",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "user",
				Labels:                 map[string]string{},
				Actions: []types.TemplateAction{
					{Label: "View workspace", URL: "https://coder.example.com/@user/my-workspace"},
					{Label: "View template", URL: "https://coder.example.com/templates/docker"},
				},
			},
			bodyTemplate: `Your workspace is ready.`,
			presentInHTML: []string{
				`href="https://coder.example.com/@user/my-workspace"`,
				"View workspace",
				`href="https://coder.example.com/templates/docker"`,
				"View template",
			},
		},
		{
			name: "footer links and logo render correctly",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "user",
				Labels:                 map[string]string{},
			},
			bodyTemplate: `Hello.`,
			presentInHTML: []string{
				// Footer links
				`href="https://coder.example.com/settings/notifications"`,
				"manage your notification settings",
				"Stop receiving emails like this",
				// Logo
				`<img src="https://coder.example.com/logo.png"`,
				// Copyright
				"Coder. All rights reserved",
			},
		},
		{
			name: "conditional label rendering",
			payload: types.MessagePayload{
				NotificationTemplateID: "00000000-0000-0000-0000-000000000000",
				UserName:               "admin",
				Labels: map[string]string{
					"created_account_name":      "newuser",
					"created_account_user_name": "New User",
					"initiator":                 "admin",
				},
			},
			bodyTemplate: `New user account **{{.Labels.created_account_name}}** has been created.` + "\n\n" +
				`This new user account was created {{if .Labels.created_account_user_name}}for **{{.Labels.created_account_user_name}}** {{end}}by **{{.Labels.initiator}}**.`,
			presentInHTML: []string{
				"<strong>newuser</strong>",
				"<strong>New User</strong>",
				"<strong>admin</strong>",
				"for",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Step 1: Render the body from a sanitized copy, leaving the
			// payload itself raw (as notifier.prepare does).
			sanitized := render.SanitizedPayload(tt.payload)

			// Step 2: Render body template via GoTemplate
			body, err := render.GoTemplate(tt.bodyTemplate, sanitized, nil)
			require.NoError(t, err)

			// Step 3: Convert markdown body to HTML (as SMTP dispatcher does)
			htmlBody := markdown.HTMLFromMarkdownSafe(body)

			// Step 4: Render subject via PlaintextFromMarkdown
			subject := "Test notification"

			// Step 5: Inject into outer HTML template using the raw payload (as
			// the SMTP dispatcher does). The greeting interpolates the raw
			// UserName, made safe by the template's `| html`, not by Markdown
			// escaping.
			tt.payload.Labels["_subject"] = subject
			tt.payload.Labels["_body"] = htmlBody
			finalHTML, err := render.GoTemplate(htmlTemplate, tt.payload, helpers)
			require.NoError(t, err)

			// Step 6: Assert on final HTML output
			for _, s := range tt.absentInHTML {
				require.False(t, strings.Contains(finalHTML, s),
					"final HTML email must NOT contain %q", s)
			}
			for _, s := range tt.presentInHTML {
				require.True(t, strings.Contains(finalHTML, s),
					"final HTML email must contain %q", s)
			}
		})
	}
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
