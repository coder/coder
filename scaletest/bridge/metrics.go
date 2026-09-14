package bridge

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	bridgeErrors      *prometheus.CounterVec
	bridgeRequests    *prometheus.CounterVec
	bridgeDuration    prometheus.Histogram
	bridgeTokensTotal *prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	errors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "scaletest",
		Name:      "bridge_errors_total",
		Help:      "Total number of bridge errors",
	}, []string{"action"})

	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "scaletest",
		Name:      "bridge_requests_total",
		Help:      "Total number of bridge requests",
	}, []string{"status"})

	duration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "coderd",
		Subsystem: "scaletest",
		Name:      "bridge_request_duration_seconds",
		Help:      "Duration of bridge requests in seconds",
		Buckets:   prometheus.DefBuckets,
	})

	tokens := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "coderd",
		Subsystem: "scaletest",
		Name:      "bridge_response_tokens_total",
		Help:      "Total number of tokens in bridge responses",
	}, []string{"type"})

	reg.MustRegister(errors, requests, duration, tokens)

	return &Metrics{
		bridgeErrors:      errors,
		bridgeRequests:    requests,
		bridgeDuration:    duration,
		bridgeTokensTotal: tokens,
	}
}

func (m *Metrics) AddError(action string) {
	m.bridgeErrors.WithLabelValues(action).Inc()
}

func (m *Metrics) AddRequest(status string) {
	m.bridgeRequests.WithLabelValues(status).Inc()
}

func (m *Metrics) ObserveDuration(duration float64) {
	m.bridgeDuration.Observe(duration)
}

func (m *Metrics) AddTokens(tokenType string, count int64) {
	m.bridgeTokensTotal.WithLabelValues(tokenType).Add(float64(count))
}
