package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/httpmw"
)

func TestPrometheus(t *testing.T) {
	t.Parallel()
	t.Run("All", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
		res := chimw.NewWrapResponseWriter(httptest.NewRecorder(), 0)
		reg := prometheus.NewRegistry()
		httpmw.Prometheus(reg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(res, req)
		metrics, err := reg.Gather()
		require.NoError(t, err)
		require.Greater(t, len(metrics), 0)
	})
}
