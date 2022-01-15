//go:build embed
// +build embed

// We use build tags so tests, linting, and other Go tooling
// can compile properly without building the site.

package site

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndexPageRenders(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(Handler())

	resp, err := srv.Client().Get(srv.URL)
	require.NoError(t, err, "get index")
	data, _ := io.ReadAll(resp.Body)
	require.NotEmpty(t, data, "index should have contents")
}
