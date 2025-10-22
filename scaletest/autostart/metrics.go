package autostart

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	AutostartJobCreationLatencySeconds prometheus.HistogramVec
	AutostartJobAcquiredLatencySeconds prometheus.HistogramVec
	AutostartTotalLatencySeconds       prometheus.HistogramVec
	AutostartErrorsTotal               prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		AutostartJobCreationLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "autostart_job_creation_latency_seconds",
			Help:      "Time from when the workspace is scheduled to be autostarted to when the autostart job has been created.",
		}, []string{"username", "workspace_name"}),
		AutostartJobAcquiredLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "autostart_job_acquired_latency_seconds",
			Help:      "Time from when the workspace is scheduled to be autostarted to when the job has been acquired by a provisioner daemon.",
		}, []string{"username", "workspace_name"}),
		AutostartTotalLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "autostart_total_latency_seconds",
			Help:      "Time from when the workspace is scheduled to be autostarted to when the autostart build has finished.",
		}, []string{"username", "workspace_name"}),
		AutostartErrorsTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "autostart_errors_total",
			Help:      "Total number of autostart errors",
		}, []string{"username", "action"}),
	}

	reg.MustRegister(m.AutostartTotalLatencySeconds)
	reg.MustRegister(m.AutostartJobCreationLatencySeconds)
	reg.MustRegister(m.AutostartJobAcquiredLatencySeconds)
	reg.MustRegister(m.AutostartErrorsTotal)
	return m
}

func (m *Metrics) RecordCompletion(elapsed time.Duration, username string, workspace string) {
	m.AutostartTotalLatencySeconds.WithLabelValues(username, workspace).Observe(elapsed.Seconds())
}

func (m *Metrics) RecordJobCreation(elapsed time.Duration, username string, workspace string) {
	m.AutostartJobCreationLatencySeconds.WithLabelValues(username, workspace).Observe(elapsed.Seconds())
}

func (m *Metrics) RecordJobAcquired(elapsed time.Duration, username string, workspace string) {
	m.AutostartJobAcquiredLatencySeconds.WithLabelValues(username, workspace).Observe(elapsed.Seconds())
}

func (m *Metrics) AddError(username string, action string) {
	m.AutostartErrorsTotal.WithLabelValues(username, action).Inc()
}
