package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/proxyhealth"
)

func TestCSP(t *testing.T) {
	t.Parallel()

	proxyHosts := []*proxyhealth.ProxyHost{
		{
			Host:    "test.com",
			AppHost: "*.test.com",
		},
		{
			Host:    "coder.com",
			AppHost: "*.coder.com",
		},
		{
			// Host is not added because it duplicates the host header.
			Host:    "example.com",
			AppHost: "*.coder2.com",
		},
	}
	expectedMedia := []string{"media.com", "media2.com"}

	expected := []string{
		"frame-src 'self' *.test.com *.coder.com *.coder2.com",
		"media-src 'self' " + strings.Join(expectedMedia, " "),
		strings.Join([]string{
			"connect-src", "'self'",
			// Added from host header.
			"wss://example.com", "ws://example.com",
			// Added via proxy hosts.
			"wss://test.com", "ws://test.com", "https://test.com", "http://test.com",
			"wss://coder.com", "ws://coder.com", "https://coder.com", "http://coder.com",
		}, " "),
	}

	// When the host is empty, it uses example.com.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rw := httptest.NewRecorder()

	httpmw.CSPHeaders(false, func() []*proxyhealth.ProxyHost {
		return proxyHosts
	}, map[httpmw.CSPFetchDirective][]string{
		httpmw.CSPDirectiveMediaSrc: expectedMedia,
	})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})).ServeHTTP(rw, r)

	require.NotEmpty(t, rw.Header().Get("Content-Security-Policy"), "Content-Security-Policy header should not be empty")
	for _, e := range expected {
		require.Contains(t, rw.Header().Get("Content-Security-Policy"), e)
	}
}
