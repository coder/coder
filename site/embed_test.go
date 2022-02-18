package site_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"github.com/coder/coder/site"
)

func TestIndexPageRenders(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(site.Handler(slog.Logger{}))
	defer srv.Close()

	ctx, cancelFunc := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancelFunc()

	// As a special case, check the root page is not cached
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	require.NoError(t, err, "create request")

	resp, err := srv.Client().Do(req)
	require.NoError(t, err, "get index")

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read response")
	require.NotEmpty(t, data, "index should have contents")
	require.NoError(t, resp.Body.Close(), "closing response")
}
