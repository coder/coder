package nextrouter_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/psanford/memfs"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/nextrouter"
)

func request(server *httptest.Server, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+path, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	return res, err
}

func TestNextRouter(t *testing.T) {
	t.Parallel()

	t.Run("Serves file at root", func(t *testing.T) {
		t.Parallel()
		rootFS := memfs.New()
		err := rootFS.WriteFile("test.html", []byte("test123"), 0755)
		require.NoError(t, err)

		router := nextrouter.Handler(rootFS)
		server := httptest.NewServer(router)

		res, err := request(server, "/test.html")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, string(body), "test123")
		require.Equal(t, res.StatusCode, 200)
	})

	t.Run("Serves html file without extension", func(t *testing.T) {
		t.Parallel()
		rootFS := memfs.New()
		err := rootFS.WriteFile("test.html", []byte("test-no-extension"), 0755)
		require.NoError(t, err)

		router := nextrouter.Handler(rootFS)
		server := httptest.NewServer(router)

		res, err := request(server, "/test")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, string(body), "test-no-extension")
		require.Equal(t, res.StatusCode, 200)
	})

	t.Run("Defaults to index.html at root", func(t *testing.T) {
		t.Parallel()
		rootFS := memfs.New()
		err := rootFS.WriteFile("index.html", []byte("test-root-index"), 0755)
		require.NoError(t, err)

		router := nextrouter.Handler(rootFS)
		server := httptest.NewServer(router)

		res, err := request(server, "/")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, string(body), "test-root-index")
		require.Equal(t, res.StatusCode, 200)
	})

	t.Run("Serves nested file", func(t *testing.T) {
		t.Parallel()

		rootFS := memfs.New()
		err := rootFS.MkdirAll("test/a/b", 0777)
		require.NoError(t, err)

		rootFS.WriteFile("test/a/b/c.html", []byte("test123"), 0755)
		require.NoError(t, err)

		router := nextrouter.Handler(rootFS)
		server := httptest.NewServer(router)

		res, err := request(server, "/test/a/b/c.html")
		require.NoError(t, err)
		defer res.Body.Close()

		res, err = request(server, "/test/a/b/c.html")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, string(body), "test123")
		require.Equal(t, res.StatusCode, 200)
	})

	t.Run("Uses index.html in nested path", func(t *testing.T) {
		t.Parallel()

		rootFS := memfs.New()
		err := rootFS.MkdirAll("test/a/b/c", 0777)
		require.NoError(t, err)

		rootFS.WriteFile("test/a/b/c/index.html", []byte("test-abc-index"), 0755)
		require.NoError(t, err)

		router := nextrouter.Handler(rootFS)

		server := httptest.NewServer(router)

		res, err := request(server, "/test/a/b/c")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, string(body), "test-abc-index")
		require.Equal(t, res.StatusCode, 200)
	})

	t.Run("404 if file at root is not found", func(t *testing.T) {
		t.Parallel()

		rootFS := memfs.New()
		err := rootFS.WriteFile("test.html", []byte("test123"), 0755)
		require.NoError(t, err)

		router := nextrouter.Handler(rootFS)
		server := httptest.NewServer(router)

		res, err := request(server, "/test-non-existent.html")
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, res.StatusCode, 404)
	})
}
