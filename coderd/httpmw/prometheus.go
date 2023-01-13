package httpmw

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/coderd/tracing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

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
		Help:      "The number of concurrent API requests.",
	})
	websocketsConcurrent := factory.NewGauge(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "concurrent_websockets",
		Help:      "The total number of concurrent API websockets.",
	})
	websocketsDist := factory.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "websocket_durations_seconds",
		Help:      "Websocket duration distribution of requests in seconds.",
		Buckets: []float64{
			0.001, // 1ms
			1,
			60,           // 1 minute
			60 * 60,      // 1 hour
			60 * 60 * 15, // 15 hours
			60 * 60 * 30, // 30 hours
		},
	}, []string{"path"})
	requestsDist := factory.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "request_latencies_seconds",
		Help:      "Latency distribution of requests in seconds.",
		Buckets:   []float64{0.001, 0.005, 0.010, 0.025, 0.050, 0.100, 0.500, 1, 5, 10, 30},
	}, []string{"method", "path"})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var (
				start  = time.Now()
				method = r.Method
				rctx   = chi.RouteContext(r.Context())
			)

			sw, ok := w.(*tracing.StatusWriter)
			if !ok {
				panic("dev error: http.ResponseWriter is not *tracing.StatusWriter")
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
			dist.WithLabelValues(distOpts...).Observe(time.Since(start).Seconds())
		})
	}
}
