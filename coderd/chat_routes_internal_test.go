package coderd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatFilesRateLimitMWCompatibilityAliases(t *testing.T) {
	t.Parallel()

	api := &API{Options: &Options{FilesRateLimit: 1}}
	handler := api.chatFilesRateLimitMW()(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusOK)
	}))

	for i, requestPath := range []string{
		"/api/v2/chats/files/00000000-0000-0000-0000-000000000000",
		"/api/experimental/chats/files/00000000-0000-0000-0000-000000000000",
	} {
		req := httptest.NewRequest(http.MethodGet, requestPath, nil)
		req.RemoteAddr = "192.0.2.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		resp := rec.Result()
		_ = resp.Body.Close()

		expectedStatus := http.StatusOK
		if i > 0 {
			expectedStatus = http.StatusTooManyRequests
		}
		require.Equal(t, expectedStatus, resp.StatusCode, requestPath)
	}
}
