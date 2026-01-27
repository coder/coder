package aibridgeproxyd

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	RequestTypeMITM     = "mitm"
	RequestTypeTunneled = "tunneled"
)

// Metrics holds all prometheus metrics for aibridgeproxyd.
type Metrics struct {
	registerer prometheus.Registerer

	// ConnectSessionsTotal counts CONNECT sessions established.
	// Labels: type (mitm/tunneled)
	ConnectSessionsTotal *prometheus.CounterVec

	// MITMRequestsTotal counts MITM requests handled by the proxy.
	// Labels: provider
	MITMRequestsTotal *prometheus.CounterVec

	// InflightMITMRequests tracks the number of MITM requests currently being processed.
	// Labels: provider
	InflightMITMRequests *prometheus.GaugeVec

	// MITMRequestDuration tracks the duration of MITM requests in seconds.
	// This measures end-to-end time from receiving the request to sending
	// the complete response back to the client.
	// Labels: provider
	MITMRequestDuration *prometheus.HistogramVec

	// MITMResponsesTotal counts MITM responses by HTTP status code class.
	// Labels: code (2XX/3XX/4XX/5XX), provider
	MITMResponsesTotal *prometheus.CounterVec
}

// NewMetrics creates and registers all metrics for aibridgeproxyd.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)

	return &Metrics{
		registerer: reg,

		ConnectSessionsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "connect_sessions_total",
			Help: "Total number of CONNECT sessions established.",
		}, []string{"type"}),

		MITMRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "mitm_requests_total",
			Help: "Total number of MITM requests handled by the proxy.",
		}, []string{"provider"}),

		InflightMITMRequests: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "inflight_mitm_requests",
			Help: "Number of MITM requests currently being processed.",
		}, []string{"provider"}),

		MITMRequestDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name: "mitm_request_duration_seconds",
			Help: "Duration of MITM requests in seconds.",
			Buckets: []float64{
				0.5, // 500ms
				1,   // 1s
				2.5, // 2.5s
				5,   // 5s
				10,  // 10s
				30,  // 30s
				60,  // 1min
				120, // 2min - long streaming responses
			},
			NativeHistogramBucketFactor: 1.1,
			// Max number of native buckets kept at once to bound memory.
			NativeHistogramMaxBucketNumber: 100,
			// Merge/flush small buckets periodically to control churn.
			NativeHistogramMinResetDuration: time.Hour,
			// Treat tiny values as zero (helps with noisy near-zero latencies).
			NativeHistogramZeroThreshold:    0,
			NativeHistogramMaxZeroThreshold: 0,
		}, []string{"provider"}),

		MITMResponsesTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "mitm_responses_total",
			Help: "Total number of MITM responses by HTTP status code class.",
		}, []string{"code", "provider"}),
	}
}

// Unregister removes all metrics from the registerer.
func (m *Metrics) Unregister() {
	m.registerer.Unregister(m.ConnectSessionsTotal)
	m.registerer.Unregister(m.MITMRequestsTotal)
	m.registerer.Unregister(m.InflightMITMRequests)
	m.registerer.Unregister(m.MITMRequestDuration)
	m.registerer.Unregister(m.MITMResponsesTotal)
}
