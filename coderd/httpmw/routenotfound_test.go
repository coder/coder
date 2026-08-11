package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
)

func TestRouteNotFound(t *testing.T) {
	t.Parallel()

	called := false
	next := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		called = true
		rw.WriteHeader(http.StatusOK)
	})

	rw := httptest.NewRecorder()
	httpmw.RouteNotFound()(next).ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/api/v2/tasks", nil))

	require.False(t, called, "the wrapped handler must not run")
	require.Equal(t, http.StatusNotFound, rw.Code)
}
