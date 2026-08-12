package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// NewUnstartedHTTPServer is httptest.NewUnstartedServer with keep-alives
// disabled by default. Tests that proxy or pool connections to a bare
// httptest.Server can intermittently fail on Windows with a bare EOF when
// a stale pooled connection is reused, because net/http does not retry a
// non-replayable request (e.g. a POST) on a closed pooled connection.
// Disabling keep-alives forces a fresh connection per request, eliminating
// that class of flake. Call Start before sending requests. See
// https://github.com/coder/internal/issues/1564 (AIGOV-430).
func NewUnstartedHTTPServer(t testing.TB, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.SetKeepAlivesEnabled(false)
	t.Cleanup(srv.Close)
	return srv
}
