package httpmw_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/httpmw"
	"github.com/coder/coder/testutil"
)

func TestRateLimit(t *testing.T) {
	t.Parallel()
	t.Run("NoUser", func(t *testing.T) {
		t.Parallel()
		rtr := chi.NewRouter()
		rtr.Use(httpmw.RateLimitPerMinute(5))
		rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
		})

		require.Eventually(t, func() bool {
			req := httptest.NewRequest("GET", "/", nil)
			rec := httptest.NewRecorder()
			rtr.ServeHTTP(rec, req)
			resp := rec.Result()
			defer resp.Body.Close()
			return resp.StatusCode == http.StatusTooManyRequests
		}, testutil.WaitShort, testutil.IntervalFast)
	})
}
