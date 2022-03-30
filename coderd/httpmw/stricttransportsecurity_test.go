package httpmw_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/httpmw"
)

const (
	strictTransportSecurityHeader = "Strict-Transport-Security"
	strictTransportSecurityMaxAge = time.Hour * 24 * 365
)

func TestStrictTransportSecurity(t *testing.T) {
	t.Parallel()

	setup := func(enable bool) *http.Response {
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)

		rtr := chi.NewRouter()
		rtr.Use(httpmw.StrictTransportSecurity(enable))
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
		require.Contains(t, res.Header.Get(strictTransportSecurityHeader), fmt.Sprintf("max-age=%d", int64(strictTransportSecurityMaxAge.Seconds())))
	})
	t.Run("False", func(t *testing.T) {
		t.Parallel()

		res := setup(false)
		defer res.Body.Close()
		require.NotContains(t, res.Header.Get(strictTransportSecurityHeader), fmt.Sprintf("max-age=%d", int64(strictTransportSecurityMaxAge.Seconds())))
		require.Equal(t, res.Header.Get(strictTransportSecurityHeader), "")
	})
}
