package nextrouter_test

import (
	"context"
	"io"
	"io/fs"
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

		router, err := nextrouter.Handler(rootFS)
		require.NoError(t, err)
		server := httptest.NewServer(router)

		res, err := request(server, "/test.html")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, string(body), "test123")
		require.Equal(t, res.StatusCode, 200)
	})

	t.Run("Serves nested file", func(t *testing.T) {
		t.Parallel()

		rootFS := memfs.New()
		err := rootFS.MkdirAll("test/a/b", 0777)
		require.NoError(t, err)

		rootFS.WriteFile("test/a/b/c.html", []byte("test123"), 0755)
		require.NoError(t, err)

		router, err := nextrouter.Handler(rootFS)
		require.NoError(t, err)
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

	t.Run("404 if file at root is not found", func(t *testing.T) {
		t.Parallel()

		rootFS := memfs.New()
		err := rootFS.WriteFile("test.html", []byte("test123"), 0755)
		require.NoError(t, err)

		router, err := nextrouter.Handler(rootFS)
		require.NoError(t, err)
		server := httptest.NewServer(router)

		res, err := request(server, "/test-non-existent.html")
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, res.StatusCode, 404)
	})

	t.Run("Smoke test", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(nil)

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, res.StatusCode, 404)

		rootFS := memfs.New()
		err = rootFS.MkdirAll("test/a/b", 0777)
		require.NoError(t, err)

		rootFS.WriteFile("test/a/b/c.txt", []byte("test123"), 0755)
		content, err := fs.ReadFile(rootFS, "test/a/b/c.txt")
		require.NoError(t, err)

		require.Equal(t, string(content), "test123")

		//require.Equal(t, 1, 2)
	})
}
