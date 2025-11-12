package prebuilds

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	PrebuildJobsCreated   prometheus.GaugeVec
	PrebuildJobsRunning   prometheus.GaugeVec
	PrebuildJobsFailed    prometheus.GaugeVec
	PrebuildJobsCompleted prometheus.GaugeVec

	PrebuildDeletionJobsCreated   prometheus.GaugeVec
	PrebuildDeletionJobsRunning   prometheus.GaugeVec
	PrebuildDeletionJobsFailed    prometheus.GaugeVec
	PrebuildDeletionJobsCompleted prometheus.GaugeVec

	PrebuildErrorsTotal prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		PrebuildJobsCreated: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_jobs_created",
			Help:      "Number of prebuild jobs that have been created.",
		}, []string{"template_name"}),
		PrebuildJobsRunning: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_jobs_running",
			Help:      "Number of prebuild jobs that are currently running.",
		}, []string{"template_name"}),
		PrebuildJobsFailed: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_jobs_failed",
			Help:      "Number of prebuild jobs that have failed.",
		}, []string{"template_name"}),
		PrebuildJobsCompleted: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_jobs_completed",
			Help:      "Number of prebuild jobs that have completed successfully.",
		}, []string{"template_name"}),
		PrebuildDeletionJobsCreated: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_deletion_jobs_created",
			Help:      "Number of prebuild deletion jobs that have been created.",
		}, []string{"template_name"}),
		PrebuildDeletionJobsRunning: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_deletion_jobs_running",
			Help:      "Number of prebuild deletion jobs that are currently running.",
		}, []string{"template_name"}),
		PrebuildDeletionJobsFailed: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_deletion_jobs_failed",
			Help:      "Number of prebuild deletion jobs that have failed.",
		}, []string{"template_name"}),
		PrebuildDeletionJobsCompleted: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_deletion_jobs_completed",
			Help:      "Number of prebuild deletion jobs that have completed successfully.",
		}, []string{"template_name"}),
		PrebuildErrorsTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "prebuild_errors_total",
			Help:      "Total number of prebuild errors",
		}, []string{"template_name", "action"}),
	}

	reg.MustRegister(m.PrebuildJobsCreated)
	reg.MustRegister(m.PrebuildJobsRunning)
	reg.MustRegister(m.PrebuildJobsFailed)
	reg.MustRegister(m.PrebuildJobsCompleted)
	reg.MustRegister(m.PrebuildDeletionJobsCreated)
	reg.MustRegister(m.PrebuildDeletionJobsRunning)
	reg.MustRegister(m.PrebuildDeletionJobsFailed)
	reg.MustRegister(m.PrebuildDeletionJobsCompleted)
	reg.MustRegister(m.PrebuildErrorsTotal)
	return m
}

func (m *Metrics) SetJobsCreated(count int, templateName string) {
	m.PrebuildJobsCreated.WithLabelValues(templateName).Set(float64(count))
}

func (m *Metrics) SetJobsRunning(count int, templateName string) {
	m.PrebuildJobsRunning.WithLabelValues(templateName).Set(float64(count))
}

func (m *Metrics) SetJobsFailed(count int, templateName string) {
	m.PrebuildJobsFailed.WithLabelValues(templateName).Set(float64(count))
}

func (m *Metrics) SetJobsCompleted(count int, templateName string) {
	m.PrebuildJobsCompleted.WithLabelValues(templateName).Set(float64(count))
}

func (m *Metrics) SetDeletionJobsCreated(count int, templateName string) {
	m.PrebuildDeletionJobsCreated.WithLabelValues(templateName).Set(float64(count))
}

func (m *Metrics) SetDeletionJobsRunning(count int, templateName string) {
	m.PrebuildDeletionJobsRunning.WithLabelValues(templateName).Set(float64(count))
}

func (m *Metrics) SetDeletionJobsFailed(count int, templateName string) {
	m.PrebuildDeletionJobsFailed.WithLabelValues(templateName).Set(float64(count))
}

func (m *Metrics) SetDeletionJobsCompleted(count int, templateName string) {
	m.PrebuildDeletionJobsCompleted.WithLabelValues(templateName).Set(float64(count))
}

func (m *Metrics) AddError(templateName string, action string) {
	m.PrebuildErrorsTotal.WithLabelValues(templateName, action).Inc()
}
