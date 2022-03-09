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

func TestIndexPageRenders(t *testing.T) {
	t.Parallel()

	rootFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("index-test-file"),
		},
	}

	srv := httptest.NewServer(site.Handler(rootFS, slog.Logger{}))

	req, err := http.NewRequestWithContext(context.Background(), "GET", srv.URL, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "get index")
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	require.Equal(t, string(data), "index-test-file")
}

func TestNestedPathsRenderIndex(t *testing.T) {
	t.Parallel()

	rootFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte("index-test-file"),
		},
	}

	srv := httptest.NewServer(site.Handler(rootFS, slog.Logger{}))

	path := srv.URL + "/some/nested/path"

	req, err := http.NewRequestWithContext(context.Background(), "GET", path, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "get index")
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	require.Equal(t, string(data), "index-test-file")
}

func TestCacheHeaderseAreCorrect(t *testing.T) {
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

	srv := httptest.NewServer(site.Handler(rootFS, slog.Logger{}))

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

/*func TestNestedPageRenders(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(site.Handler(slog.Logger{}))

	req, err := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/some/random/path", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "get index for random path")
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	require.NotEmpty(t, data, "index should have contents")
}*/
