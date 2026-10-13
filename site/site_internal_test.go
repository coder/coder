package site

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestResolveThemes(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		settings  codersdk.UserAppearanceSettings
		wantLight string
		wantDark  string
	}{
		{
			name:      "Unset follows the OS",
			wantLight: "light",
			wantDark:  "dark",
		},
		{
			name:      "Single theme",
			settings:  codersdk.UserAppearanceSettings{ThemeMode: codersdk.ThemeModeSingle, ThemePreference: "light-tritan"},
			wantLight: "light-tritan",
			wantDark:  "light-tritan",
		},
		{
			name:      "Legacy concrete preference",
			settings:  codersdk.UserAppearanceSettings{ThemePreference: "dark"},
			wantLight: "dark",
			wantDark:  "dark",
		},
		{
			name: "Sync",
			settings: codersdk.UserAppearanceSettings{
				ThemeMode:  codersdk.ThemeModeSync,
				ThemeLight: "light-protan-deuter",
				ThemeDark:  "dark-tritan",
			},
			wantLight: "light-protan-deuter",
			wantDark:  "dark-tritan",
		},
		{
			name:      "Sync with invalid themes falls back per slot",
			settings:  codersdk.UserAppearanceSettings{ThemeMode: codersdk.ThemeModeSync, ThemeLight: "bogus"},
			wantLight: "light",
			wantDark:  "dark",
		},
		{
			name:      "Legacy auto",
			settings:  codersdk.UserAppearanceSettings{ThemePreference: "auto-tritan"},
			wantLight: "light-tritan",
			wantDark:  "dark-tritan",
		},
		{
			name:      "Unknown preference follows the OS",
			settings:  codersdk.UserAppearanceSettings{ThemeMode: codersdk.ThemeModeSingle, ThemePreference: "bogus"},
			wantLight: "light",
			wantDark:  "dark",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			light, dark := resolveThemes(tc.settings)
			assert.Equal(t, tc.wantLight, light)
			assert.Equal(t, tc.wantDark, dark)
		})
	}
}

func TestThemeClass(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "dark", themeClass("dark"))
	assert.Equal(t, "light", themeClass("light"))
	assert.Equal(t, "dark-tritan dark", themeClass("dark-tritan"))
	assert.Equal(t, "light-protan-deuter light", themeClass("light-protan-deuter"))
}

func TestStylesheetHrefs(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<!doctype html>
<html><head>
<meta property="user" content="{{ .User }}" />
<link rel="icon" href="/favicons/favicon.svg" />
<link rel="modulepreload" href="/assets/index-a.js">
<link rel="stylesheet" crossorigin href="/assets/monaco-b.css">
<link rel="stylesheet" crossorigin href="/assets/index-c.css">
<link rel="stylesheet" href="https://example.com/evil.css">
</head><body></body></html>`)},
	}
	assert.Equal(t, []string{"/assets/monaco-b.css", "/assets/index-c.css"}, stylesheetHrefs(fsys))
	assert.Empty(t, stylesheetHrefs(fstest.MapFS{}))
}

func TestRenderOAuthAllowPageTheme(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oauth2/authorize", nil)
	RenderOAuthAllowPage(rec, req, RenderOAuthAllowData{
		AppName:      "test-app",
		Username:     "alice",
		Unrestricted: true,
		Appearance: codersdk.UserAppearanceSettings{
			ThemeMode:  codersdk.ThemeModeSync,
			ThemeLight: "light-tritan",
			ThemeDark:  "dark-tritan",
		},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `class="dark-tritan dark"`)
	assert.Contains(t, body, `data-theme-light="light-tritan light"`)
	assert.Contains(t, body, `data-theme-dark="dark-tritan dark"`)
}
