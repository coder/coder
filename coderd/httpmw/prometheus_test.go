package httpmw_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	cm "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
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

	t.Run("Concurrent", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		reg := prometheus.NewRegistry()
		promMW := httpmw.Prometheus(reg)

		// Create a test handler to simulate a WebSocket connection
		testHandler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(rw, r, nil)
			if !assert.NoError(t, err, "failed to accept websocket") {
				return
			}
			defer conn.Close(websocket.StatusGoingAway, "")
		})

		wrappedHandler := promMW(testHandler)

		r := chi.NewRouter()
		r.Use(tracing.StatusWriterMiddleware, promMW)
		r.Get("/api/v2/build/{build}/logs", func(rw http.ResponseWriter, r *http.Request) {
			wrappedHandler.ServeHTTP(rw, r)
		})

		srv := httptest.NewServer(r)
		defer srv.Close()
		// nolint: bodyclose
		conn, _, err := websocket.Dial(ctx, srv.URL+"/api/v2/build/1/logs", nil)
		require.NoError(t, err, "failed to dial WebSocket")
		defer conn.Close(websocket.StatusNormalClosure, "")

		metrics, err := reg.Gather()
		require.NoError(t, err)
		require.Greater(t, len(metrics), 0)
		metricLabels := getMetricLabels(metrics)

		concurrentWebsockets, ok := metricLabels["coderd_api_concurrent_websockets"]
		require.True(t, ok, "coderd_api_concurrent_websockets metric not found")
		require.Equal(t, "/api/v2/build/{build}/logs", concurrentWebsockets["path"])
	})

	t.Run("UserRoute", func(t *testing.T) {
		t.Parallel()
		reg := prometheus.NewRegistry()
		promMW := httpmw.Prometheus(reg)

		r := chi.NewRouter()
		r.With(promMW).Get("/api/v2/users/{user}", func(w http.ResponseWriter, r *http.Request) {})

		req := httptest.NewRequest("GET", "/api/v2/users/john", nil)

		sw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}

		r.ServeHTTP(sw, req)

		metrics, err := reg.Gather()
		require.NoError(t, err)
		require.Greater(t, len(metrics), 0)
		metricLabels := getMetricLabels(metrics)

		reqProcessed, ok := metricLabels["coderd_api_requests_processed_total"]
		require.True(t, ok, "coderd_api_requests_processed_total metric not found")
		require.Equal(t, "/api/v2/users/{user}", reqProcessed["path"])
		require.Equal(t, "GET", reqProcessed["method"])

		concurrentRequests, ok := metricLabels["coderd_api_concurrent_requests"]
		require.True(t, ok, "coderd_api_concurrent_requests metric not found")
		require.Equal(t, "/api/v2/users/{user}", concurrentRequests["path"])
		require.Equal(t, "GET", concurrentRequests["method"])
	})
}

func getMetricLabels(metrics []*cm.MetricFamily) map[string]map[string]string {
	metricLabels := map[string]map[string]string{}
	for _, metricFamily := range metrics {
		metricName := metricFamily.GetName()
		metricLabels[metricName] = map[string]string{}
		for _, metric := range metricFamily.GetMetric() {
			for _, labelPair := range metric.GetLabel() {
				metricLabels[metricName][labelPair.GetName()] = labelPair.GetValue()
			}
		}
	}
	return metricLabels
}
