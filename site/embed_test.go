//go:build !slim
// +build !slim

package site_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"

	"github.com/coder/coder/site"
)

func TestReturnsIndexPageForNestedPaths(t *testing.T) {
	t.Parallel()

	rootFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("index-test-file"),
		},
		"favicon.ico": &fstest.MapFile{
			Data: []byte("favicon-bytes"),
		},
	}

	var nestedPathTests = []struct {
		path     string
		expected string
	}{
		// HTML cases
		{"/index.html", "index-test-file"},
		{"/", "index-test-file"},
		{"/nested/index.html", "index-test-file"},
		{"/nested", "index-test-file"},
		{"/nested/", "index-test-file"},
		{"/double/nested/index.html", "index-test-file"},
		{"/double/nested", "index-test-file"},
		{"/double/nested/", "index-test-file"},

		// Other file cases
		{"/favicon.ico", "favicon-bytes"},
		// Ensure that nested still picks up the 'top-level' file
		{"/nested/favicon.ico", "favicon-bytes"},
		{"/double/nested/favicon.ico", "favicon-bytes"},
	}

	srv := httptest.NewServer(site.Handler(rootFS, slog.Logger{}, defaultTemplateFunc))

	for _, testCase := range nestedPathTests {
		path := srv.URL + testCase.path

		req, err := http.NewRequestWithContext(context.Background(), "GET", path, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err, "get index")
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		require.Equal(t, string(data), testCase.expected)
	}
}

func TestCacheHeadersAreCorrect(t *testing.T) {
	rootFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("index-test-file"),
		},
		"favicon.ico": &fstest.MapFile{
			Data: []byte("favicon-bytes"),
		},
		"bundle.js": &fstest.MapFile{
			Data: []byte("bundle-js-bytes"),
		},
		"icon.svg": &fstest.MapFile{
			Data: []byte("svg-bytes"),
		},
	}

	srv := httptest.NewServer(site.Handler(rootFS, slog.Logger{}, defaultTemplateFunc))

	dynamicPaths := []string{
		"/",
		"/index.html",
		"/some/random/path",
		"/some/random/path/",
		"/some/random/path/index.html",
	}

	cachedPaths := []string{
		"/favicon.ico",
		"/bundle.js",
		"/icon.svg",
	}

	for _, path := range cachedPaths {
		ctx, cancelFunc := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancelFunc()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/"+path, nil)
		require.NoError(t, err, "create request")

		resp, err := srv.Client().Do(req)
		require.NoError(t, err, "get index")

		cache := resp.Header.Get("Cache-Control")
		require.Equalf(t, cache, "public, max-age=31536000, immutable", "expected path %q to have immutable cache", path)
		require.NoError(t, resp.Body.Close(), "closing response")
	}

	for _, path := range dynamicPaths {
		ctx, cancelFunc := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancelFunc()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/"+path, nil)
		require.NoError(t, err, "create request")

		resp, err := srv.Client().Do(req)
		require.NoError(t, err, "get index")

		cache := resp.Header.Get("Cache-Control")
		require.Emptyf(t, cache, "expected path %q to be un-cacheable", path)
		require.NoError(t, resp.Body.Close(), "closing response")
	}
}

func TestTemplateParametersAreInjected(t *testing.T) {
	rootFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("{{ .CSP.Nonce }} | {{ .CSRF.Token }}"),
		},
		// Template parameters should  only be injected in HTML,
		// so this provides a negative case
		"bundle.js": &fstest.MapFile{
			Data: []byte("{{ .CSP.Nonce }}"),
		},
	}

	templateFunc := func(r *http.Request) site.HtmlState {
		return site.HtmlState{
			CSP:  site.CSPState{Nonce: "test-nonce"},
			CSRF: site.CSRFState{Token: "test-token"},
		}
	}

	srv := httptest.NewServer(site.Handler(rootFS, slog.Logger{}, templateFunc))

	var testCases = []struct {
		path             string
		expectedContents string
	}{
		// Rendered HTML cases
		{"/index.html", "test-nonce | test-token"},
		{"/nested/index.html", "test-nonce | test-token"},
		{"/nested/", "test-nonce | test-token"},

		// Non-HTML cases (template should not render)
		{"/bundle.js", "{{ .CSP.Nonce }}"},
	}

	for _, testCase := range testCases {

		ctx, cancelFunc := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancelFunc()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/"+testCase.path, nil)
		require.NoError(t, err, "create request")

		resp, err := srv.Client().Do(req)
		require.NoError(t, err, "get index")

		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		require.Equal(t, string(data), testCase.expectedContents)
	}
}

func TestReturnsErrorIfNoIndex(t *testing.T) {
	rootFS := fstest.MapFS{
		// No index.html - so our router will have no fallback!
		"favicon.ico": &fstest.MapFile{
			Data: []byte("favicon-bytes"),
		},
		"bundle.js": &fstest.MapFile{
			Data: []byte("bundle-js-bytes"),
		},
		"icon.svg": &fstest.MapFile{
			Data: []byte("svg-bytes"),
		},
	}

	// When no index.html is available, the site handler should panic
	require.Panics(t, func() {
		site.Handler(rootFS, slog.Logger{}, defaultTemplateFunc)
	})
}

func defaultTemplateFunc(r *http.Request) site.HtmlState {
	return site.HtmlState{
		CSP:  site.CSPState{Nonce: "test-csp-nonce"},
		CSRF: site.CSRFState{Token: "test-csrf-token"},
	}
}
