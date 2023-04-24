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

	"github.com/coder/coder/coderd/coderdtest"
	"github.com/coder/coder/coderd/healthcheck"
)

func TestAccessURL(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithCancel(context.Background())
			report      healthcheck.AccessURLReport
			client      = coderdtest.New(t, nil)
		)
		defer cancel()

		report.Run(ctx, &healthcheck.AccessURLOptions{
			AccessURL: client.URL,
		})

		assert.True(t, report.Healthy)
		assert.True(t, report.Reachable)
		assert.Equal(t, http.StatusOK, report.StatusCode)
		assert.Equal(t, "OK", report.HealthzResponse)
		assert.NoError(t, report.Err)
	})

	t.Run("404", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithCancel(context.Background())
			report      healthcheck.AccessURLReport
			resp        = []byte("NOT OK")
			srv         = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write(resp)
			}))
		)
		defer cancel()
		defer srv.Close()

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)

		report.Run(ctx, &healthcheck.AccessURLOptions{
			Client:    srv.Client(),
			AccessURL: u,
		})

		assert.False(t, report.Healthy)
		assert.True(t, report.Reachable)
		assert.Equal(t, http.StatusNotFound, report.StatusCode)
		assert.Equal(t, string(resp), report.HealthzResponse)
		assert.NoError(t, report.Err)
	})

	t.Run("ClientErr", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithCancel(context.Background())
			report      healthcheck.AccessURLReport
			resp        = []byte("OK")
			srv         = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write(resp)
			}))
			client = srv.Client()
		)
		defer cancel()
		defer srv.Close()

		expErr := xerrors.New("client error")
		client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, expErr
		})

		u, err := url.Parse(srv.URL)
		require.NoError(t, err)

		report.Run(ctx, &healthcheck.AccessURLOptions{
			Client:    client,
			AccessURL: u,
		})

		assert.False(t, report.Healthy)
		assert.False(t, report.Reachable)
		assert.Equal(t, 0, report.StatusCode)
		assert.Equal(t, "", report.HealthzResponse)
		assert.ErrorIs(t, report.Err, expErr)
	})
}

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (rt roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt(r)
}
