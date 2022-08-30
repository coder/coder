package httpmw

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/coderd/httpapi"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func durationToFloatMs(d time.Duration) float64 {
	return float64(d.Milliseconds())
}

func Prometheus(register prometheus.Registerer) func(http.Handler) http.Handler {
	factory := promauto.With(register)
	requestsProcessed := factory.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "requests_processed_total",
		Help:      "The total number of processed API requests",
	}, []string{"code", "method", "path"})
	requestsConcurrent := factory.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "concurrent_requests",
		Help:      "The number of concurrent API requests",
	})
	websocketsConcurrent := factory.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "concurrent_websockets",
		Help:      "The total number of concurrent API websockets",
	})
	websocketsDist := factory.NewHistogramVec(prometheus.HistogramOpts{
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
	requestsDist := factory.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "request_latencies_ms",
		Help:      "Latency distribution of requests in milliseconds",
		Buckets:   []float64{1, 5, 10, 25, 50, 100, 500, 1000, 5000, 10000, 30000},
	}, []string{"method", "path"})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var (
				start  = time.Now()
				method = r.Method
				rctx   = chi.RouteContext(r.Context())
			)

			sw, ok := w.(*httpapi.StatusWriter)
			if !ok {
				panic("dev error: http.ResponseWriter is not *httpapi.StatusWriter")
			}

			var (
				dist     *prometheus.HistogramVec
				distOpts []string
			)
			// We want to count WebSockets separately.
			if httpapi.IsWebsocketUpgrade(r) {
				websocketsConcurrent.Inc()
				defer websocketsConcurrent.Dec()

				dist = websocketsDist
			} else {
				requestsConcurrent.Inc()
				defer requestsConcurrent.Dec()

				dist = requestsDist
				distOpts = []string{method}
			}

			next.ServeHTTP(w, r)

			path := rctx.RoutePattern()
			distOpts = append(distOpts, path)
			statusStr := strconv.Itoa(sw.Status)

			requestsProcessed.WithLabelValues(statusStr, method, path).Inc()
			dist.WithLabelValues(distOpts...).Observe(float64(time.Since(start)) / 1e6)
		})
	}
}
