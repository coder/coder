package oauth2provider_test

import (
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
