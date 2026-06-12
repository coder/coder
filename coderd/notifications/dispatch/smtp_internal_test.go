package dispatch

import (
	"html"
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
