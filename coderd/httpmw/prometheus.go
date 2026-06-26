package httpmw

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/tracing"
)

// WSMetrics groups all WebSocket-related Prometheus metrics so they
// can be created once and shared between the HTTP middleware and the
// WSWatcher probe recorder.
type WSMetrics struct {
	Concurrent *prometheus.GaugeVec
	Durations  *prometheus.HistogramVec
	Probes     *prometheus.CounterVec
}

// NewWSMetrics registers and returns WebSocket metrics. The returned
// struct is safe to pass to both Prometheus() and
// WSMetrics.RecordProbe.
func NewWSMetrics(reg prometheus.Registerer) *WSMetrics {
	factory := promauto.With(reg)
	return &WSMetrics{
		Concurrent: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "api",
			Name:      "concurrent_websockets",
			Help:      "The total number of concurrent API websockets.",
		}, []string{"path"}),
		Durations: factory.NewHistogramVec(prometheus.HistogramOpts{
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
		}, []string{"path"}),
		Probes: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "api",
			Name:      "websocket_probes_total",
			Help: "WebSocket liveness probe outcomes by route. " +
				"Compare rate(...{result=\"ok\"}[1m]) against " +
				"coderd_api_concurrent_websockets to detect " +
				"unresponsive WebSocket connections.",
		}, []string{"path", "result"}),
	}
}

// RecordProbe records a single liveness probe outcome. It extracts
// the HTTP route from ctx via ExtractHTTPRoute.
func (m *WSMetrics) RecordProbe(ctx context.Context, r httpapi.ProbeResult) {
	m.Probes.WithLabelValues(ExtractHTTPRoute(ctx), string(r)).Inc()
}

func Prometheus(register prometheus.Registerer, ws *WSMetrics) func(http.Handler) http.Handler {
	if ws == nil {
		panic("developer error: WSMetrics is nil")
	}
	factory := promauto.With(register)
	requestsProcessed := factory.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "requests_processed_total",
		Help:      "The total number of processed API requests",
	}, []string{"code", "method", "path"})
	requestsConcurrent := factory.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "coderd",
		Subsystem: "api",
		Name:      "concurrent_requests",
		Help:      "The number of concurrent API requests.",
	}, []string{"method", "path"})
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
			)

			sw, ok := w.(*tracing.StatusWriter)
			if !ok {
				panic("dev error: http.ResponseWriter is not *tracing.StatusWriter")
			}

			var (
				dist     *prometheus.HistogramVec
				distOpts []string
				path     = ExtractHTTPRoute(r.Context())
			)

			// We want to count WebSockets separately.
			if httpapi.IsWebsocketUpgrade(r) {
				ws.Concurrent.WithLabelValues(path).Inc()
				defer ws.Concurrent.WithLabelValues(path).Dec()

				dist = ws.Durations
			} else {
				requestsConcurrent.WithLabelValues(method, path).Inc()
				defer requestsConcurrent.WithLabelValues(method, path).Dec()

				dist = requestsDist
				distOpts = []string{method}
			}

			next.ServeHTTP(w, r)

			distOpts = append(distOpts, path)
			statusStr := strconv.Itoa(sw.Status)

			requestsProcessed.WithLabelValues(statusStr, method, path).Inc()
			dist.WithLabelValues(distOpts...).Observe(time.Since(start).Seconds())
		})
	}
}
