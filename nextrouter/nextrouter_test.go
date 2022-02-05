package nextrouter_test

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/psanford/memfs"
	"github.com/stretchr/testify/require"
)

func TestConn(t *testing.T) {
	t.Parallel()

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
