package aibridged

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics is the prometheus surface for aibridged provider reloads.
type Metrics struct {
	registerer prometheus.Registerer

	// ProviderInfo is one series per configured provider; value is
	// always 1 and the status label carries the alertable signal.
	// Labels: provider_name, provider_type, status.
	ProviderInfo *prometheus.GaugeVec

	// ProvidersLastReloadTimestampSeconds is the unix timestamp of the
	// last reload attempt, success or failure.
	ProvidersLastReloadTimestampSeconds prometheus.Gauge

	// ProvidersLastReloadSuccessTimestampSeconds is the unix timestamp
	// of the last reload that successfully refreshed the pool. A gap
	// against ProvidersLastReloadTimestampSeconds means the loop is
	// firing but the refresh function is failing.
	ProvidersLastReloadSuccessTimestampSeconds prometheus.Gauge
}

// NewMetrics registers the provider metrics against reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)

	return &Metrics{
		registerer: reg,

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
			Help: "Unix timestamp of the last provider reload that successfully refreshed the pool. A gap against the providers_last_reload_timestamp_seconds gauge means the loop is firing but the refresh function is failing.",
		}),
	}
}

// Unregister removes the provider metrics from the registerer.
func (m *Metrics) Unregister() {
	if m == nil {
		return
	}
	m.registerer.Unregister(m.ProviderInfo)
	m.registerer.Unregister(m.ProvidersLastReloadTimestampSeconds)
	m.registerer.Unregister(m.ProvidersLastReloadSuccessTimestampSeconds)
}

// RecordReloadAttempt stamps the attempt-time gauge at the start of a
// reload. A reload that hangs mid-flight is detected by watching the
// gap between this gauge and ProvidersLastReloadSuccessTimestampSeconds.
func (m *Metrics) RecordReloadAttempt() {
	if m == nil {
		return
	}
	m.ProvidersLastReloadTimestampSeconds.Set(float64(time.Now().Unix()))
}

// RecordReloadSuccess rewrites the ProviderInfo GaugeVec from the
// outcomes and stamps the success-time gauge. Reset clears series for
// providers that have left the configuration so they don't linger as
// stale.
func (m *Metrics) RecordReloadSuccess(outcomes []ProviderOutcome) {
	if m == nil {
		return
	}
	WriteProviderInfoSnapshot(m.ProviderInfo, outcomes)
	m.ProvidersLastReloadSuccessTimestampSeconds.Set(float64(time.Now().Unix()))
}

// WriteProviderInfoSnapshot Resets info and writes one series per
// outcome. Both aibridged and aibridgeproxyd use this so the
// provider_info recording contract stays in one place.
func WriteProviderInfoSnapshot(info *prometheus.GaugeVec, outcomes []ProviderOutcome) {
	info.Reset()
	for _, o := range outcomes {
		info.WithLabelValues(o.Name, o.Type, string(o.Status)).Set(1)
	}
}
