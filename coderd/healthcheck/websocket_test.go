package healthcheck_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/healthcheck"
	"github.com/coder/coder/testutil"
)

func TestWebsocket(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(&healthcheck.WebsocketEchoServer{})
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)

		wsReport := healthcheck.WebsocketReport{}
		wsReport.Run(ctx, &healthcheck.WebsocketReportOptions{
			AccessURL:  u,
			HTTPClient: srv.Client(),
			APIKey:     "test",
		})

		require.NoError(t, wsReport.Error)
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(&healthcheck.WebsocketEchoServer{
			Error: xerrors.New("test error"),
			Code:  http.StatusBadRequest,
		})
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)

		wsReport := healthcheck.WebsocketReport{}
		wsReport.Run(ctx, &healthcheck.WebsocketReportOptions{
			AccessURL:  u,
			HTTPClient: srv.Client(),
			APIKey:     "test",
		})

		require.Error(t, wsReport.Error)
		assert.Equal(t, wsReport.Response.Body, "test error")
		assert.Equal(t, wsReport.Response.Code, http.StatusBadRequest)
	})
}
