package httpmw_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	cm "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/httpapi"
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
		httpmw.HTTPRoute(httpmw.Prometheus(reg, httpmw.NewWSMetrics(reg))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))).ServeHTTP(res, req)
		metrics, err := reg.Gather()
		require.NoError(t, err)
		require.Greater(t, len(metrics), 0)
	})

	t.Run("Concurrent", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
		defer cancel()

		reg := prometheus.NewRegistry()
		promMW := httpmw.Prometheus(reg, httpmw.NewWSMetrics(reg))

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
		r.Use(tracing.StatusWriterMiddleware, httpmw.HTTPRoute, promMW)
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
		promMW := httpmw.Prometheus(reg, httpmw.NewWSMetrics(reg))

		r := chi.NewRouter()
		r.With(httpmw.HTTPRoute).With(promMW).Get("/api/v2/users/{user}", func(w http.ResponseWriter, r *http.Request) {})

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

	t.Run("StaticRoute", func(t *testing.T) {
		t.Parallel()
		reg := prometheus.NewRegistry()
		promMW := httpmw.Prometheus(reg, httpmw.NewWSMetrics(reg))

		r := chi.NewRouter()
		r.Use(httpmw.HTTPRoute)
		r.Use(promMW)
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		r.Get("/static/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/static/bundle.js", nil)
		sw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}

		r.ServeHTTP(sw, req)

		metrics, err := reg.Gather()
		require.NoError(t, err)
		require.Greater(t, len(metrics), 0)
		metricLabels := getMetricLabels(metrics)

		reqProcessed, ok := metricLabels["coderd_api_requests_processed_total"]
		require.True(t, ok, "coderd_api_requests_processed_total metric not found")
		require.Equal(t, "STATIC", reqProcessed["path"])
		require.Equal(t, "GET", reqProcessed["method"])
	})

	// An oversized body is counted per route so that a limit set too tight for a
	// legitimate payload is visible without waiting for a user report. The
	// counter is keyed on the response status, so it covers handlers that bound
	// their own bodies as well as those going through httpapi.Read, and the
	// reason label separates a body size rejection from the other reasons coderd
	// answers 413.
	t.Run("RequestTooLarge", func(t *testing.T) {
		t.Parallel()
		reg := prometheus.NewRegistry()
		promMW := httpmw.Prometheus(reg, httpmw.NewWSMetrics(reg))

		r := chi.NewRouter()
		r.Use(httpmw.HTTPRoute)
		r.Use(promMW)
		// A body size rejection records the limit that tripped.
		r.Post("/api/v2/users/{user}/secrets/batch", func(w http.ResponseWriter, r *http.Request) {
			httpapi.RecordRequestBodyLimit(r.Context(), 8<<20)
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		})
		// Agent log storage overflow answers 413 for a reason that has nothing
		// to do with the request body, and must not be attributed to one.
		r.Post("/api/v2/workspaceagents/me/logs", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		})
		r.Post("/api/v2/users/{user}/secrets", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})

		for _, path := range []string{
			"/api/v2/users/john/secrets/batch",
			"/api/v2/workspaceagents/me/logs",
			// A route that does not reject must not be counted.
			"/api/v2/users/john/secrets",
		} {
			sw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}
			r.ServeHTTP(sw, httptest.NewRequest("POST", path, nil))
		}

		metrics, err := reg.Gather()
		require.NoError(t, err)

		counts := map[string]float64{}
		var found bool
		for _, family := range metrics {
			if family.GetName() != "coderd_api_requests_too_large_total" {
				continue
			}
			found = true
			for _, metric := range family.GetMetric() {
				labels := map[string]string{}
				for _, pair := range metric.GetLabel() {
					labels[pair.GetName()] = pair.GetValue()
				}
				require.Equal(t, "POST", labels["method"])
				counts[labels["path"]+" "+labels["reason"]] = metric.GetCounter().GetValue()
			}
		}
		require.True(t, found, "coderd_api_requests_too_large_total metric not found")

		require.Equal(t, map[string]float64{
			"/api/v2/users/{user}/secrets/batch request_body": 1,
			"/api/v2/workspaceagents/me/logs other":           1,
		}, counts)
	})

	t.Run("UnknownRoute", func(t *testing.T) {
		t.Parallel()
		reg := prometheus.NewRegistry()
		promMW := httpmw.Prometheus(reg, httpmw.NewWSMetrics(reg))

		r := chi.NewRouter()
		r.Use(httpmw.HTTPRoute)
		r.Use(promMW)
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		r.Get("/api/v2/users/{user}", func(w http.ResponseWriter, r *http.Request) {})

		req := httptest.NewRequest("GET", "/api/v2/weird_path", nil)
		sw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}

		r.ServeHTTP(sw, req)

		metrics, err := reg.Gather()
		require.NoError(t, err)
		require.Greater(t, len(metrics), 0)
		metricLabels := getMetricLabels(metrics)

		reqProcessed, ok := metricLabels["coderd_api_requests_processed_total"]
		require.True(t, ok, "coderd_api_requests_processed_total metric not found")
		require.Equal(t, "UNKNOWN", reqProcessed["path"])
		require.Equal(t, "GET", reqProcessed["method"])
	})

	t.Run("Subrouter", func(t *testing.T) {
		t.Parallel()
		reg := prometheus.NewRegistry()
		promMW := httpmw.Prometheus(reg, httpmw.NewWSMetrics(reg))

		r := chi.NewRouter()
		r.Use(httpmw.HTTPRoute)
		r.Use(promMW)
		r.Get("/api/v2/workspaceagents/{workspaceagent}/pty", func(w http.ResponseWriter, r *http.Request) {})

		// Mount under a root router like wsproxy does.
		rootRouter := chi.NewRouter()
		rootRouter.Get("/latency-check", func(w http.ResponseWriter, r *http.Request) {})
		rootRouter.Mount("/", r)

		agentID := uuid.UUID{1}
		req := httptest.NewRequest("GET", fmt.Sprintf("/api/v2/workspaceagents/%s/pty", agentID.String()), nil)

		sw := &tracing.StatusWriter{ResponseWriter: httptest.NewRecorder()}
		rootRouter.ServeHTTP(sw, req)

		metrics, err := reg.Gather()
		require.NoError(t, err)
		require.Greater(t, len(metrics), 0)
		metricLabels := getMetricLabels(metrics)

		reqProcessed, ok := metricLabels["coderd_api_requests_processed_total"]
		require.True(t, ok, "coderd_api_requests_processed_total metric not found")
		require.Equal(t, "/api/v2/workspaceagents/{workspaceagent}/pty", reqProcessed["path"])
		require.Equal(t, "GET", reqProcessed["method"])

		concurrentRequests, ok := metricLabels["coderd_api_concurrent_requests"]
		require.True(t, ok, "coderd_api_concurrent_requests metric not found")
		require.Equal(t, "/api/v2/workspaceagents/{workspaceagent}/pty", concurrentRequests["path"])
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
