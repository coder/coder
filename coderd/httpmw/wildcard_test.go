package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/httpmw"
)

func TestWildcard(t *testing.T) {
	t.Parallel()
	t.Run("Match", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "http://frogs.bananas.org", nil)
		res := httptest.NewRecorder()
		httpmw.Wildcard("bananas.org", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})).ServeHTTP(res, req)
		require.Equal(t, http.StatusOK, res.Result().StatusCode)
	})

	t.Run("Passthrough", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "http://frogs.apples.org", nil)
		res := httptest.NewRecorder()
		httpmw.Wildcard("bananas.org", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})).ServeHTTP(res, req)
		require.Equal(t, http.StatusForbidden, res.Result().StatusCode)
	})
}
