package httpmw

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	requestsProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "requests_processed_total",
		Help:      "The total number of processed API requests",
	}, []string{"code", "method", "path"})
	requestsConcurrent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "concurrent_requests",
		Help:      "The number of concurrent API requests",
	}, []string{"method", "path"})
	websocketsConcurrent = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "concurrent_websockets",
		Help:      "The total number of concurrent API websockets",
	}, []string{"path"})
	websocketsDist = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "websocket_durations_ms",
		Help:      "Websocket duration distribution of requests in milliseconds",
		Buckets: []float64{
			durationToFloatMs(01 * time.Millisecond),
			durationToFloatMs(01 * time.Second),
			durationToFloatMs(01 * time.Minute),
			durationToFloatMs(01 * time.Hour),
			durationToFloatMs(15 * time.Hour),
			durationToFloatMs(30 * time.Hour),
		},
	}, []string{"path"})
	requestsDist = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "request_latencies_ms",
		Help:      "Latency distribution of requests in milliseconds",
		Buckets:   []float64{1, 5, 10, 25, 50, 100, 500, 1000, 5000, 10000, 30000},
	}, []string{"method", "path"})
)

func durationToFloatMs(d time.Duration) float64 {
	return float64(d.Milliseconds())
}

func Prometheus(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			start  = time.Now()
			method = r.Method
			rctx   = chi.RouteContext(r.Context())
			path   = rctx.RoutePattern()
		)
		sw, ok := w.(middleware.WrapResponseWriter)
		if !ok {
			panic("dev error: http.ResponseWriter is not middleware.WrapResponseWriter")
		}

		var (
			dist     *prometheus.HistogramVec
			distOpts []string
		)
		// We want to count websockets separately.
		if isWebsocketUpgrade(r) {
			websocketsConcurrent.WithLabelValues(path).Inc()
			defer websocketsConcurrent.WithLabelValues(path).Dec()

			dist = websocketsDist
			distOpts = []string{path}
		} else {
			requestsConcurrent.WithLabelValues(method, path).Inc()
			defer requestsConcurrent.WithLabelValues(method, path).Dec()

			dist = requestsDist
			distOpts = []string{method, path}
		}

		next.ServeHTTP(w, r)
		statusStr := strconv.Itoa(sw.Status())

		requestsProcessed.WithLabelValues(statusStr, method, path).Inc()
		dist.WithLabelValues(distOpts...).Observe(float64(time.Since(start).Milliseconds()))
	})
}

func isWebsocketUpgrade(r *http.Request) bool {
	vs := r.Header.Values("Upgrade")
	for _, v := range vs {
		if v == "websocket" {
			return true
		}
	}
	return false
}
