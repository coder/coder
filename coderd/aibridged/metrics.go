package aibridged

import (
	"strconv"
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

	// PolicyPipelineInfo is one series per provider with an active policy
	// pipeline; value is always 1 and the pipeline_version label carries the
	// live version so operators can confirm what is enforced after a swap.
	// Labels: provider_name, pipeline_version.
	PolicyPipelineInfo *prometheus.GaugeVec

	// PolicyPipelineReloadsTotal counts policy pipeline snapshot reloads by
	// result (success or failure). A rising failure count with a stale snapshot
	// is the keep-last-good divergence signal.
	PolicyPipelineReloadsTotal *prometheus.CounterVec

	// PolicyPipelineDrift is the number of active-pipeline memberships pinned to
	// a non-current policy version (drift). Nonzero means a pipeline is running
	// an out-of-date policy version.
	PolicyPipelineDrift prometheus.Gauge
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
			Help: "Unix timestamp of the last provider reload that successfully refreshed the pool. A gap against coder_aibridged_providers_last_reload_timestamp_seconds means the loop is firing but the refresh function is failing.",
		}),

		PolicyPipelineInfo: factory.NewGaugeVec(prometheus.GaugeOpts{
			Name: "policy_pipeline_info",
			Help: "One series per provider with an active policy pipeline. Value is always 1; the pipeline_version label carries the live version.",
		}, []string{"provider_name", "pipeline_version"}),

		PolicyPipelineReloadsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "policy_pipeline_reloads_total",
			Help: "Count of AI gateway policy pipeline snapshot reloads by result (success or failure).",
		}, []string{"result"}),

		PolicyPipelineDrift: factory.NewGauge(prometheus.GaugeOpts{
			Name: "policy_pipeline_drift",
			Help: "Number of active-pipeline memberships pinned to a non-current policy version.",
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
	m.registerer.Unregister(m.PolicyPipelineInfo)
	m.registerer.Unregister(m.PolicyPipelineReloadsTotal)
	m.registerer.Unregister(m.PolicyPipelineDrift)
}

// PipelinePolicyOutcome summarizes one provider's active policy pipeline for
// the reload metrics.
type PipelinePolicyOutcome struct {
	Provider        string
	PipelineVersion int32
}

// RecordPipelineReloadSuccess rewrites the PolicyPipelineInfo gauge from the
// outcomes and bumps the success counter.
func (m *Metrics) RecordPipelineReloadSuccess(outcomes []PipelinePolicyOutcome) {
	if m == nil {
		return
	}
	m.PolicyPipelineReloadsTotal.WithLabelValues("success").Inc()
	m.PolicyPipelineInfo.Reset()
	for _, o := range outcomes {
		m.PolicyPipelineInfo.WithLabelValues(o.Provider, strconv.Itoa(int(o.PipelineVersion))).Set(1)
	}
}

// RecordPipelineReloadFailure bumps the failure counter, leaving the
// PolicyPipelineInfo gauge (and the live snapshot) unchanged.
func (m *Metrics) RecordPipelineReloadFailure() {
	if m == nil {
		return
	}
	m.PolicyPipelineReloadsTotal.WithLabelValues("failure").Inc()
}

// SetPolicyDrift sets the drift gauge to n.
func (m *Metrics) SetPolicyDrift(n int) {
	if m == nil {
		return
	}
	m.PolicyPipelineDrift.Set(float64(n))
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
