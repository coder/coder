package oauth2provider_test

import (
	htmltemplate "html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/site"
)

func TestOAuthConsentFormIncludesCSRFToken(t *testing.T) {
	t.Parallel()

	const csrfFieldValue = "csrf-field-value"
	req := httptest.NewRequest(http.MethodGet, "https://coder.com/oauth2/authorize", nil)
	rec := httptest.NewRecorder()

	site.RenderOAuthAllowPage(rec, req, site.RenderOAuthAllowData{
		AppName:     "Test OAuth App",
		CancelURI:   "https://coder.com/cancel",
		RedirectURI: "https://coder.com/oauth2/authorize?client_id=test",
		CSRFToken:   csrfFieldValue,
		Username:    "test-user",
	})

	require.Equal(t, http.StatusOK, rec.Result().StatusCode)
	assert.Contains(t, rec.Body.String(), `name="csrf_token"`)
	assert.Contains(t, rec.Body.String(), `value="`+csrfFieldValue+`"`)
}

func TestOAuthConsentCancelButtonRendersURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cancelURI string
	}{
		{
			name:      "StandardHTTPS",
			cancelURI: "https://example.com/callback?error=access_denied",
		},
		{
			name:      "Localhost",
			cancelURI: "http://127.0.0.1:9090/callback?error=access_denied",
		},
		{
			name:      "CustomScheme",
			cancelURI: "custom-scheme://app/callback?error=access_denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "https://coder.com/oauth2/authorize", nil)
			rec := httptest.NewRecorder()

			site.RenderOAuthAllowPage(rec, req, site.RenderOAuthAllowData{
				AppName:     "Test App",
				CancelURI:   htmltemplate.URL(tt.cancelURI),
				RedirectURI: "https://coder.com/oauth2/authorize?client_id=test",
				CSRFToken:   "token",
				Username:    "test-user",
			})

			require.Equal(t, http.StatusOK, rec.Result().StatusCode)
			body := rec.Body.String()
			// The Cancel link must contain the actual URI, not a
			// sanitized placeholder like "#ZgotmplZ".
			assert.Contains(t, body, tt.cancelURI)
			assert.NotContains(t, body, "#ZgotmplZ")
		})
	}
}
