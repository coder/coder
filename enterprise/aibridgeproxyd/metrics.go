package aibridgeproxyd

import (
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

	// MITMResponsesTotal counts MITM responses by HTTP status code.
	// Labels: code (HTTP status code), provider
	// Cardinality is bounded: ~100 used status codes x few providers.
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
	m.registerer.Unregister(m.MITMResponsesTotal)
}
