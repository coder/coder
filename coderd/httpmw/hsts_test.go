package httpmw_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/httpmw"
)

func TestHSTS(t *testing.T) {
	t.Parallel()

	setup := func(enable bool) *http.Response {
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)

		rtr := chi.NewRouter()
		rtr.Use(httpmw.HSTS(enable))
		rtr.Get("/", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello!"))
		})
		rtr.ServeHTTP(rw, r)
		return rw.Result()
	}

	t.Run("True", func(t *testing.T) {
		t.Parallel()

		res := setup(true)
		defer res.Body.Close()
		require.Contains(t, res.Header.Get(httpmw.HSTSHeader), fmt.Sprintf("max-age=%d", int64(httpmw.HSTSMaxAge)))
	})
	t.Run("False", func(t *testing.T) {
		t.Parallel()

		res := setup(false)
		defer res.Body.Close()
		require.NotContains(t, res.Header.Get(httpmw.HSTSHeader), fmt.Sprintf("max-age=%d", int64(httpmw.HSTSMaxAge)))
	})
}
