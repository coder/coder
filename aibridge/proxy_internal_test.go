package aibridge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
)

// Disabled providers return 503 without reading the body, even if it is oversized.
func TestProxyRouterDisabledProviderOversizedBody(t *testing.T) {
	t.Parallel()

	router, err := NewProxyRouter(
		[]Provider{NewDisabledProviderStub("disabled-openai", "openai")},
		slogtest.Make(t, nil),
	)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/disabled-openai/v1/chat/completions", strings.NewReader("x"))
	req.ContentLength = maxRequestBodyBytes + 1

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Contains(t, rec.Body.String(), ErrorCodeProviderDisabled)
}
