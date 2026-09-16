package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// NewHTTPTestServer return a *httptest.Server with the following
// defaults set:
//   - keep-alives disabled by default to prevent stale connection reuse (AIGOV-430).
//
// Override these defaults via opts if needed.
// The server is started and will be closed when the test ends.
func NewHTTPTestServer(t testing.TB, handler http.Handler, opts ...func(*httptest.Server)) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.SetKeepAlivesEnabled(false)
	for _, opt := range opts {
		opt(srv)
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}
