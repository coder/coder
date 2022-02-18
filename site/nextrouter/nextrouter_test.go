package nextrouter_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/psanford/memfs"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"

	"github.com/coder/coder/site/nextrouter"
)

func TestNextRouter(t *testing.T) {
	t.Parallel()

	t.Run("Serves file at root", func(t *testing.T) {
		t.Parallel()
		rootFS := memfs.New()
		err := rootFS.WriteFile("test.html", []byte("test123"), 0755)
		require.NoError(t, err)

		router, err := nextrouter.Handler(rootFS, nil)
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

	// This is a test case for the issue we hit in V1 w/ NextJS migration
	t.Run("Prefer file over folder w/ trailing slash", func(t *testing.T) {
		t.Parallel()
		rootFS := memfs.New()
		err := rootFS.MkdirAll("folder", 0777)
		require.NoError(t, err)
		err = rootFS.WriteFile("folder.html", []byte("folderFile"), 0755)
		require.NoError(t, err)

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)
		server := httptest.NewServer(router)

		res, err := request(server, "/folder/")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, string(body), "folderFile")
		require.Equal(t, res.StatusCode, 200)
	})

	t.Run("Serves non-html files at root", func(t *testing.T) {
		t.Parallel()
		rootFS := memfs.New()
		err := rootFS.WriteFile("test.png", []byte("png-bytes"), 0755)
		require.NoError(t, err)

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)
		server := httptest.NewServer(router)

		res, err := request(server, "/test.png")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, res.Header.Get("Content-Type"), "image/png")
		require.Equal(t, string(body), "png-bytes")
		require.Equal(t, res.StatusCode, 200)
	})

	t.Run("Serves html file without extension", func(t *testing.T) {
		t.Parallel()
		rootFS := memfs.New()
		err := rootFS.WriteFile("test.html", []byte("test-no-extension"), 0755)
		require.NoError(t, err)

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)
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

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)
		server := httptest.NewServer(router)

		res, err := request(server, "/")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, res.Header.Get("Content-Type"), "text/html; charset=utf-8")
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

		router, err := nextrouter.Handler(rootFS, nil)
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

	t.Run("Uses index.html in nested path", func(t *testing.T) {
		t.Parallel()

		rootFS := memfs.New()
		err := rootFS.MkdirAll("test/a/b/c", 0777)
		require.NoError(t, err)

		rootFS.WriteFile("test/a/b/c/index.html", []byte("test-abc-index"), 0755)
		require.NoError(t, err)

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)
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

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)
		server := httptest.NewServer(router)

		res, err := request(server, "/test-non-existent.html")
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, res.StatusCode, 404)
	})

	t.Run("404 if file at root is not found", func(t *testing.T) {
		t.Parallel()

		rootFS := memfs.New()
		err := rootFS.WriteFile("test.html", []byte("test123"), 0755)
		require.NoError(t, err)

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)
		server := httptest.NewServer(router)

		res, err := request(server, "/test-non-existent.html")
		require.NoError(t, err)
		defer res.Body.Close()
		require.Equal(t, res.StatusCode, 404)
	})

	t.Run("Serve custom 404.html if available", func(t *testing.T) {
		t.Parallel()

		rootFS := memfs.New()
		err := rootFS.WriteFile("404.html", []byte("404 custom content"), 0755)
		require.NoError(t, err)

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)
		server := httptest.NewServer(router)

		res, err := request(server, "/test-non-existent.html")
		require.NoError(t, err)
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, res.StatusCode, 404)
		require.Equal(t, string(body), "404 custom content")
	})

	t.Run("Serves dynamic-routed file", func(t *testing.T) {
		t.Parallel()
		rootFS := memfs.New()
		err := rootFS.MkdirAll("folder", 0777)
		require.NoError(t, err)
		err = rootFS.WriteFile("folder/[orgs].html", []byte("test-dynamic-path"), 0755)
		require.NoError(t, err)

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)
		server := httptest.NewServer(router)

		res, err := request(server, "/folder/org-1")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, string(body), "test-dynamic-path")
		require.Equal(t, res.StatusCode, 200)
	})

	t.Run("Handles dynamic-routed folders", func(t *testing.T) {
		t.Parallel()
		rootFS := memfs.New()
		err := rootFS.MkdirAll("folder/[org]/[project]", 0777)
		require.NoError(t, err)
		err = rootFS.WriteFile("folder/[org]/[project]/create.html", []byte("test-create"), 0755)
		require.NoError(t, err)

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)
		server := httptest.NewServer(router)

		res, err := request(server, "/folder/org-1/project-1/create")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, string(body), "test-create")
		require.Equal(t, res.StatusCode, 200)
	})

	t.Run("Handles catch-all routes", func(t *testing.T) {
		t.Parallel()
		rootFS := memfs.New()
		err := rootFS.MkdirAll("folder", 0777)
		require.NoError(t, err)
		err = rootFS.WriteFile("folder/[[...any]].html", []byte("test-catch-all"), 0755)
		require.NoError(t, err)

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)
		server := httptest.NewServer(router)

		res, err := request(server, "/folder/org-1/project-1/random")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, string(body), "test-catch-all")
		require.Equal(t, res.StatusCode, 200)
	})

	t.Run("Static routes should be preferred to dynamic routes", func(t *testing.T) {
		t.Parallel()
		rootFS := memfs.New()
		err := rootFS.MkdirAll("folder", 0777)
		require.NoError(t, err)
		err = rootFS.WriteFile("folder/[orgs].html", []byte("test-dynamic-path"), 0755)
		require.NoError(t, err)
		err = rootFS.WriteFile("folder/create.html", []byte("test-create"), 0755)
		require.NoError(t, err)

		router, err := nextrouter.Handler(rootFS, nil)
		require.NoError(t, err)
		server := httptest.NewServer(router)

		res, err := request(server, "/folder/create")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, string(body), "test-create")
		require.Equal(t, res.StatusCode, 200)
	})

	t.Run("Injects template parameters", func(t *testing.T) {
		t.Parallel()

		rootFS := memfs.New()
		err := rootFS.WriteFile("test.html", []byte("{{ .CSRF.Token }}"), 0755)
		require.NoError(t, err)

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

		res, err := request(server, "/test.html")
		require.NoError(t, err)
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, string(body), "hello-csrf")
		require.Equal(t, res.StatusCode, 200)
	})
}

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
