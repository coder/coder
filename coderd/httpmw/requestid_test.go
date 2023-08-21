package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
)

func TestRequestID(t *testing.T) {
	t.Parallel()

	rtr := chi.NewRouter()
	rtr.Use(httpmw.AttachRequestID)
	rtr.Get("/", func(w http.ResponseWriter, r *http.Request) {
		rid := httpmw.RequestID(r)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(rid.String()))
	})
	r := httptest.NewRequest("GET", "/", nil)
	rw := httptest.NewRecorder()
	rtr.ServeHTTP(rw, r)

	res := rw.Result()
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.NotEmpty(t, res.Header.Get("X-Coder-Request-ID"))
	require.NotEmpty(t, rw.Body.Bytes())
}
