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

func TestCSPFrameAncestors(t *testing.T) {
	t.Parallel()

	t.Run("DefaultSelf", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rw := httptest.NewRecorder()

		httpmw.CSPHeaders(false, func() []*proxyhealth.ProxyHost {
			return nil
		}, nil)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})).ServeHTTP(rw, r)

		csp := rw.Header().Get("Content-Security-Policy")
		require.Contains(t, csp, "frame-ancestors 'self'")
	})

	t.Run("OverrideViaStaticAdditions", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rw := httptest.NewRecorder()

		httpmw.CSPHeaders(false, func() []*proxyhealth.ProxyHost {
			return nil
		}, map[httpmw.CSPFetchDirective][]string{
			httpmw.CSPFrameAncestors: {"https://example.com"},
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})).ServeHTTP(rw, r)

		csp := rw.Header().Get("Content-Security-Policy")
		require.Contains(t, csp, "frame-ancestors https://example.com")
		require.NotContains(t, csp, "frame-ancestors 'self'")
	})

	t.Run("OmitWhenEmpty", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rw := httptest.NewRecorder()

		httpmw.CSPHeaders(false, func() []*proxyhealth.ProxyHost {
			return nil
		}, map[httpmw.CSPFetchDirective][]string{
			httpmw.CSPFrameAncestors: {},
		})(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})).ServeHTTP(rw, r)

		csp := rw.Header().Get("Content-Security-Policy")
		require.NotContains(t, csp, "frame-ancestors")
	})
}

func TestCSPSkipsInvalidProxyHosts(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	rw := httptest.NewRecorder()

	httpmw.CSPHeaders(false, func() []*proxyhealth.ProxyHost {
		return []*proxyhealth.ProxyHost{
			{Host: "proxy1.example;script-src-elem", AppHost: "*.proxy1.example"},
			{Host: "proxy2.example", AppHost: "*.proxy2.example,script-src-elem"},
			{Host: "proxy3.example *", AppHost: "*.proxy3.example"},
			{Host: "prøxy4.example", AppHost: "*.prøxy4.example"},
		}
	}, nil)(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})).ServeHTTP(rw, r)

	csp := rw.Header().Get("Content-Security-Policy")
	require.Contains(t, csp, "frame-src 'self' *.proxy1.example proxy2.example *.proxy3.example;")
	require.Contains(t, csp, strings.Join([]string{
		"connect-src", "'self'",
		"wss://example.com", "ws://example.com",
		"wss://proxy2.example", "ws://proxy2.example", "https://proxy2.example", "http://proxy2.example;",
	}, " "))
	require.NotContains(t, csp, "script-src-elem")
	require.NotContains(t, csp, "proxy3.example *")
	require.NotContains(t, csp, "prøxy4.example")
}

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
		{
			// Host is added to frame-src once because it equals the app host.
			Host:    "nowildcard.com",
			AppHost: "nowildcard.com",
		},
	}
	expectedMedia := []string{"media.com", "media2.com"}

	expected := []string{
		"frame-src 'self' *.test.com test.com *.coder.com coder.com *.coder2.com nowildcard.com;",
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
