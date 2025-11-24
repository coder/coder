package taskstatus

import "github.com/prometheus/client_golang/prometheus"

type Metrics struct {
	TaskStatusToWorkspaceUpdateLatencySeconds prometheus.HistogramVec
	MissingStatusUpdatesTotal                 prometheus.CounterVec
	ReportTaskStatusErrorsTotal               prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer, labelNames ...string) *Metrics {
	m := &Metrics{
		TaskStatusToWorkspaceUpdateLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "task_status_to_workspace_update_latency_seconds",
			Help:      "Time in seconds between reporting a task status and receiving the workspace update.",
		}, labelNames),
		MissingStatusUpdatesTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "missing_status_updates_total",
			Help:      "Total number of missing status updates.",
		}, labelNames),
		ReportTaskStatusErrorsTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "report_task_status_errors_total",
			Help:      "Total number of errors when reporting task status.",
		}, labelNames),
	}
	reg.MustRegister(m.TaskStatusToWorkspaceUpdateLatencySeconds)
	reg.MustRegister(m.MissingStatusUpdatesTotal)
	reg.MustRegister(m.ReportTaskStatusErrorsTotal)
	return m
}
