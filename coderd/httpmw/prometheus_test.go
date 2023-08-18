package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/tracing"
)

func TestPrometheus(t *testing.T) {
	t.Parallel()
	t.Run("All", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
		res := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}
		reg := prometheus.NewRegistry()
		httpmw.Prometheus(reg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(res, req)
		metrics, err := reg.Gather()
		require.NoError(t, err)
		require.Greater(t, len(metrics), 0)
	})
}
