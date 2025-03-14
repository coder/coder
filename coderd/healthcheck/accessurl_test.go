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

)
func TestAccessURL(t *testing.T) {
	t.Parallel()
	t.Run("OK", func(t *testing.T) {

		t.Parallel()
		var (
			ctx, cancel = context.WithCancel(context.Background())

			report      healthcheck.AccessURLReport
			resp        = []byte("OK")
			srv         = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(resp)
			}))
		)
		defer cancel()
		report.Run(ctx, &healthcheck.AccessURLReportOptions{
			Client:    srv.Client(),
			AccessURL: mustURL(t, srv.URL),
		})
		assert.True(t, report.Healthy)
		assert.True(t, report.Reachable)

		assert.Equal(t, health.SeverityOK, report.Severity)
		assert.Equal(t, http.StatusOK, report.StatusCode)
		assert.Equal(t, "OK", report.HealthzResponse)
		assert.Nil(t, report.Error)
	})

	t.Run("NotSet", func(t *testing.T) {
		t.Parallel()
		var (
			ctx, cancel = context.WithCancel(context.Background())
			report      healthcheck.AccessURLReport
		)
		defer cancel()
		report.Run(ctx, &healthcheck.AccessURLReportOptions{

			Client:    nil, // defaults to http.DefaultClient
			AccessURL: nil,
		})

		assert.False(t, report.Healthy)
		assert.False(t, report.Reachable)
		assert.Equal(t, health.SeverityError, report.Severity)
		assert.Equal(t, 0, report.StatusCode)
		assert.Equal(t, "", report.HealthzResponse)
		require.NotNil(t, report.Error)

		assert.Contains(t, *report.Error, health.CodeAccessURLNotSet)
	})
	t.Run("ClientErr", func(t *testing.T) {
		t.Parallel()
		var (

			ctx, cancel = context.WithCancel(context.Background())
			report      healthcheck.AccessURLReport
			resp        = []byte("OK")
			srv         = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(resp)
			}))
			client = srv.Client()
		)

		defer cancel()
		defer srv.Close()
		expErr := errors.New("client error")

		client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, expErr
		})
		report.Run(ctx, &healthcheck.AccessURLReportOptions{
			Client:    client,
			AccessURL: mustURL(t, srv.URL),
		})
		assert.False(t, report.Healthy)
		assert.False(t, report.Reachable)
		assert.Equal(t, health.SeverityError, report.Severity)
		assert.Equal(t, 0, report.StatusCode)
		assert.Equal(t, "", report.HealthzResponse)
		require.NotNil(t, report.Error)

		assert.Contains(t, *report.Error, expErr.Error())
		assert.Contains(t, *report.Error, health.CodeAccessURLFetch)
	})
	t.Run("404", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithCancel(context.Background())
			report      healthcheck.AccessURLReport
			resp        = []byte("NOT OK")
			srv         = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write(resp)
			}))
		)
		defer cancel()
		defer srv.Close()
		report.Run(ctx, &healthcheck.AccessURLReportOptions{
			Client:    srv.Client(),
			AccessURL: mustURL(t, srv.URL),
		})

		assert.False(t, report.Healthy)
		assert.True(t, report.Reachable)
		assert.Equal(t, health.SeverityWarning, report.Severity)

		assert.Equal(t, http.StatusNotFound, report.StatusCode)
		assert.Equal(t, string(resp), report.HealthzResponse)
		assert.Nil(t, report.Error)
		if assert.NotEmpty(t, report.Warnings) {
			assert.Equal(t, report.Warnings[0].Code, health.CodeAccessURLNotOK)
		}
	})
	t.Run("DismissedError", func(t *testing.T) {
		t.Parallel()
		var (
			ctx, cancel = context.WithCancel(context.Background())
			report      healthcheck.AccessURLReport

		)
		defer cancel()
		report.Run(ctx, &healthcheck.AccessURLReportOptions{
			Dismissed: true,
		})

		assert.True(t, report.Dismissed)
		assert.Equal(t, health.SeverityError, report.Severity)
	})
}
type roundTripFunc func(r *http.Request) (*http.Response, error)
func (rt roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt(r)
}
func mustURL(t testing.TB, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)

	require.NoError(t, err)
	return u
}
