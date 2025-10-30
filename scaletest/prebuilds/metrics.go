package prebuilds

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	PrebuildJobCreationLatencySeconds prometheus.HistogramVec
	PrebuildJobAcquiredLatencySeconds prometheus.HistogramVec
	PrebuildTotalLatencySeconds       prometheus.HistogramVec

	PrebuildDeletionJobCreationLatencySeconds prometheus.HistogramVec
	PrebuildDeletionJobAcquiredLatencySeconds prometheus.HistogramVec
	PrebuildDeletionTotalLatencySeconds       prometheus.HistogramVec

	PrebuildErrorsTotal prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		PrebuildJobCreationLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_job_creation_latency_seconds",
			Help:      "Time from when prebuilds are unpaused to when all the template build jobs have been created.",
		}, []string{"template_name"}),
		PrebuildJobAcquiredLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_job_acquired_latency_seconds",
			Help:      "Time from when prebuilds are unpaused to when all the prebuild jobs have been acquired by a provisioner daemon.",
		}, []string{"template_name"}),
		PrebuildTotalLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_total_latency_seconds",
			Help:      "Time from when prebuilds are unpaused to when all the prebuild builds have finished.",
		}, []string{"template_name"}),
		PrebuildDeletionJobCreationLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_deletion_job_creation_latency_seconds",
			Help:      "Time from when prebuilds are resumed for deletion to when all the deletion jobs have been created.",
		}, []string{"template_name"}),
		PrebuildDeletionJobAcquiredLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_deletion_job_acquired_latency_seconds",
			Help:      "Time from when prebuilds are resumed for deletion to when all the deletion jobs have been acquired by a provisioner daemon.",
		}, []string{"template_name"}),
		PrebuildDeletionTotalLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_deletion_total_latency_seconds",
			Help:      "Time from when prebuilds are resumed for deletion to when all the prebuild workspaces have been deleted.",
		}, []string{"template_name"}),
		PrebuildErrorsTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_errors_total",
			Help:      "Total number of prebuild errors",
		}, []string{"template_name", "action"}),
	}

	reg.MustRegister(m.PrebuildTotalLatencySeconds)
	reg.MustRegister(m.PrebuildJobCreationLatencySeconds)
	reg.MustRegister(m.PrebuildJobAcquiredLatencySeconds)
	reg.MustRegister(m.PrebuildDeletionTotalLatencySeconds)
	reg.MustRegister(m.PrebuildDeletionJobCreationLatencySeconds)
	reg.MustRegister(m.PrebuildDeletionJobAcquiredLatencySeconds)
	reg.MustRegister(m.PrebuildErrorsTotal)
	return m
}

func (m *Metrics) RecordCompletion(elapsed time.Duration, templateName string) {
	m.PrebuildTotalLatencySeconds.WithLabelValues(templateName).Observe(elapsed.Seconds())
}

func (m *Metrics) RecordJobCreation(elapsed time.Duration, templateName string) {
	m.PrebuildJobCreationLatencySeconds.WithLabelValues(templateName).Observe(elapsed.Seconds())
}

func (m *Metrics) RecordJobAcquired(elapsed time.Duration, templateName string) {
	m.PrebuildJobAcquiredLatencySeconds.WithLabelValues(templateName).Observe(elapsed.Seconds())
}

func (m *Metrics) RecordDeletionCompletion(elapsed time.Duration, templateName string) {
	m.PrebuildDeletionTotalLatencySeconds.WithLabelValues(templateName).Observe(elapsed.Seconds())
}

func (m *Metrics) RecordDeletionJobCreation(elapsed time.Duration, templateName string) {
	m.PrebuildDeletionJobCreationLatencySeconds.WithLabelValues(templateName).Observe(elapsed.Seconds())
}

func (m *Metrics) RecordDeletionJobAcquired(elapsed time.Duration, templateName string) {
	m.PrebuildDeletionJobAcquiredLatencySeconds.WithLabelValues(templateName).Observe(elapsed.Seconds())
}

func (m *Metrics) AddError(templateName string, action string) {
	m.PrebuildErrorsTotal.WithLabelValues(templateName, action).Inc()
}
