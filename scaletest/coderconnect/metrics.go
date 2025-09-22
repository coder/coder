package coderconnect

import (
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	WorkspaceUpdatesLatencySeconds prometheus.HistogramVec
	WorkspaceUpdatesErrorsTotal    prometheus.CounterVec

	numErrors          atomic.Int64
	completionDuration time.Duration
}

func NewMetrics(reg prometheus.Registerer, labelNames ...string) *Metrics {
	m := &Metrics{
		WorkspaceUpdatesLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "workspace_updates_latency_seconds",
			Help:      "Time until all expected workspaces and agents are seen via workspace updates",
		}, labelNames),
		WorkspaceUpdatesErrorsTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "workspace_updates_errors_total",
			Help:      "Total number of workspace updates errors",
		}, append(labelNames, "action")),
	}

	reg.MustRegister(m.WorkspaceUpdatesLatencySeconds)
	reg.MustRegister(m.WorkspaceUpdatesErrorsTotal)
	return m
}

func (m *Metrics) AddError(labelValues ...string) {
	m.numErrors.Add(1)
	m.WorkspaceUpdatesErrorsTotal.WithLabelValues(labelValues...).Inc()
}

func (m *Metrics) RecordCompletion(elapsed time.Duration, labelValues ...string) {
	m.completionDuration = elapsed
	m.WorkspaceUpdatesLatencySeconds.WithLabelValues(labelValues...).Observe(elapsed.Seconds())
}
