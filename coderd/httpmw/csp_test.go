package httpmw_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
)

func TestCSPConnect(t *testing.T) {
	t.Parallel()

	expected := []string{"example.com", "coder.com"}
	expectedMedia := []string{"media.com", "media2.com"}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rw := httptest.NewRecorder()

	httpmw.CSPHeaders(false, func() []string {
		return expected
	}, map[httpmw.CSPFetchDirective][]string{
		httpmw.CSPDirectiveMediaSrc: expectedMedia,
	})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})).ServeHTTP(rw, r)

	require.NotEmpty(t, rw.Header().Get("Content-Security-Policy"), "Content-Security-Policy header should not be empty")
	for _, e := range expected {
		require.Containsf(t, rw.Header().Get("Content-Security-Policy"), fmt.Sprintf("ws://%s", e), "Content-Security-Policy header should contain ws://%s", e)
		require.Containsf(t, rw.Header().Get("Content-Security-Policy"), fmt.Sprintf("wss://%s", e), "Content-Security-Policy header should contain wss://%s", e)
	}
	for _, e := range expectedMedia {
		require.Containsf(t, rw.Header().Get("Content-Security-Policy"), e, "Content-Security-Policy header should contain %s", e)
	}
}
