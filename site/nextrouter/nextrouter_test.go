package nextrouter_test

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

	"github.com/coder/coder/site/nextrouter"
)

func TestNextRouter(t *testing.T) {
	t.Parallel()

	t.Run("Serves file at root", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"test.html": &fstest.MapFile{
				Data: []byte("test123"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/test.html")
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.EqualValues(t, "test123", body)
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	// This is a test case for the issue we hit in V1 w/ NextJS migration
	t.Run("Prefer file over folder w/ trailing slash", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"folder/test.html": &fstest.MapFile{},
			"folder.html": &fstest.MapFile{
				Data: []byte("folderFile"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)
		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/folder/")
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.EqualValues(t, "folderFile", body)
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("Serves non-html files at root", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"test.png": &fstest.MapFile{
				Data: []byte("png-bytes"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/test.png")
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.Equal(t, "image/png", res.Header.Get("Content-Type"))
		require.EqualValues(t, "png-bytes", body)
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("Serves html file without extension", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"test.html": &fstest.MapFile{
				Data: []byte("test-no-extension"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/test")
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.EqualValues(t, "test-no-extension", body)
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("Defaults to index.html at root", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"index.html": &fstest.MapFile{
				Data: []byte("test-root-index"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/")
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.Equal(t, "text/html; charset=utf-8", res.Header.Get("Content-Type"))
		require.EqualValues(t, "test-root-index", body)
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("Serves nested file", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"test/a/b/c.html": &fstest.MapFile{
				Data: []byte("test123"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/test/a/b/c.html")
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())

		res, err = request(server, "/test/a/b/c.html")
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.EqualValues(t, "test123", body)
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("Uses index.html in nested path", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"test/a/b/c/index.html": &fstest.MapFile{
				Data: []byte("test-abc-index"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/test/a/b/c")
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.EqualValues(t, "test-abc-index", body)
		require.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("404 if file at root is not found", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"test.html": &fstest.MapFile{
				Data: []byte("test123"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/test-non-existent.html")
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("404 if file at root is not found", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"test.html": &fstest.MapFile{
				Data: []byte("test123"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/test-non-existent.html")
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		require.Equal(t, http.StatusNotFound, res.StatusCode)
	})

	t.Run("Serve custom 404.html if available", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"404.html": &fstest.MapFile{
				Data: []byte("404 custom content"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/test-non-existent.html")
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())

		require.Equal(t, http.StatusNotFound, res.StatusCode)
		require.EqualValues(t, "404 custom content", body)
	})

	t.Run("Serves dynamic-routed file", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"folder/[orgs].html": &fstest.MapFile{
				Data: []byte("test-dynamic-path"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/folder/org-1")
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())

		require.Equal(t, http.StatusOK, res.StatusCode)
		require.EqualValues(t, "test-dynamic-path", body)
	})

	t.Run("Handles dynamic-routed folders", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"folder/[org]/[project]/create.html": &fstest.MapFile{
				Data: []byte("test-create"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/folder/org-1/project-1/create")
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())

		require.Equal(t, http.StatusOK, res.StatusCode)
		require.EqualValues(t, "test-create", body)
	})

	t.Run("Handles catch-all routes", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"folder/[[...any]].html": &fstest.MapFile{
				Data: []byte("test-catch-all"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/folder/org-1/project-1/random")
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())

		require.Equal(t, http.StatusOK, res.StatusCode)
		require.EqualValues(t, "test-catch-all", body)
	})

	t.Run("Static routes should be preferred to dynamic routes", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"folder/[orgs].html": &fstest.MapFile{
				Data: []byte("test-dynamic-path"),
			},
			"folder/create.html": &fstest.MapFile{
				Data: []byte("test-create"),
			},
		}

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/folder/create")
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())

		require.Equal(t, http.StatusOK, res.StatusCode)
		require.EqualValues(t, "test-create", body)
	})

	t.Run("Injects template parameters", func(t *testing.T) {
		t.Parallel()

		rootFS := fstest.MapFS{
			"test.html": &fstest.MapFile{
				Data: []byte("{{ .CSRF.Token }}"),
			},
		}

		type csrfState struct {
			Token string
		}

		type template struct {
			CSRF csrfState
		}

		// Add custom template function
		templateFunc := func(request *http.Request) interface{} {
			return template{
				CSRF: csrfState{
					Token: "hello-csrf",
				},
			}
		}

		router, err := nextrouter.Handler(rootFS, &nextrouter.Options{
			Logger:           slog.Logger{},
			TemplateDataFunc: templateFunc,
		})
		require.NoError(t, err)

		server := httptest.NewServer(router)
		t.Cleanup(server.Close)

		res, err := request(server, "/test.html")
		require.NoError(t, err)

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())

		require.Equal(t, http.StatusOK, res.StatusCode)
		require.EqualValues(t, "hello-csrf", body)
	})
}

func request(server *httptest.Server, path string) (*http.Response, error) {
	ctx, cancelFunc := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelFunc()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+path, nil)
	if err != nil {
		return nil, err
	}

	res, err := server.Client().Do(req)
	if err != nil {
		return nil, err
	}
	return res, err
}
