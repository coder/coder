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

	// ProviderInfo is one series per configured provider; value is
	// always 1 and the status label carries the alertable signal.
	// Labels: provider_name, provider_type, status.
	ProviderInfo *prometheus.GaugeVec

	// ProvidersLastReloadTimestampSeconds is the unix timestamp of the
	// last reload attempt, success or failure.
	ProvidersLastReloadTimestampSeconds prometheus.Gauge

	// ProvidersLastReloadSuccessTimestampSeconds is the unix timestamp
	// of the last reload that successfully refreshed the router. A gap
	// against ProvidersLastReloadTimestampSeconds means the loop is
	// firing but the refresh function is failing.
	ProvidersLastReloadSuccessTimestampSeconds prometheus.Gauge
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

		ProviderInfo: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "provider_info",
			Help: "One series per configured AI provider. Value is always 1; the status label (enabled, disabled, error) carries the alertable signal.",
		}, []string{"provider_name", "provider_type", "status"}),

		ProvidersLastReloadTimestampSeconds: factory.NewGauge(prometheus.GaugeOpts{
			Name: "providers_last_reload_timestamp_seconds",
			Help: "Unix timestamp of the last provider reload attempt, success or failure.",
		}),

		ProvidersLastReloadSuccessTimestampSeconds: factory.NewGauge(prometheus.GaugeOpts{
			Name: "providers_last_reload_success_timestamp_seconds",
			Help: "Unix timestamp of the last provider reload that successfully refreshed the router. A gap against the providers_last_reload_timestamp_seconds gauge means the loop is firing but the refresh function is failing.",
		}),
	}
}

// Unregister removes all metrics from the registerer.
func (m *Metrics) Unregister() {
	m.registerer.Unregister(m.ConnectSessionsTotal)
	m.registerer.Unregister(m.MITMRequestsTotal)
	m.registerer.Unregister(m.InflightMITMRequests)
	m.registerer.Unregister(m.MITMResponsesTotal)
	m.registerer.Unregister(m.ProviderInfo)
	m.registerer.Unregister(m.ProvidersLastReloadTimestampSeconds)
	m.registerer.Unregister(m.ProvidersLastReloadSuccessTimestampSeconds)
}
