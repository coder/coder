package healthcheck_test

import (
	"errors"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/coder/coder/v2/coderd/healthcheck"
	"github.com/coder/coder/v2/coderd/healthcheck/health"

	"github.com/coder/coder/v2/testutil"
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
		require.Nil(t, wsReport.Error)
	})

	t.Run("Error", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(&healthcheck.WebsocketEchoServer{
			Error: errors.New("test error"),
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
		if assert.NotNil(t, wsReport.Error) {
			assert.Contains(t, *wsReport.Error, health.CodeWebsocketDial)

		}
		require.Equal(t, health.SeverityError, wsReport.Severity)
		assert.Equal(t, wsReport.Body, "test error")

		assert.Equal(t, wsReport.Code, http.StatusBadRequest)
	})
	t.Run("DismissedError", func(t *testing.T) {

		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()
		wsReport := healthcheck.WebsocketReport{}
		wsReport.Run(ctx, &healthcheck.WebsocketReportOptions{
			AccessURL: &url.URL{Host: "fake"},
			Dismissed: true,

		})
		require.True(t, wsReport.Dismissed)
		require.Equal(t, health.SeverityError, wsReport.Severity)
		require.NotNil(t, wsReport.Error)
		require.Equal(t, health.SeverityError, wsReport.Severity)
	})
}
