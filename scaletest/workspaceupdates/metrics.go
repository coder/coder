package workspaceupdates

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	WorkspaceUpdatesLatencySeconds prometheus.HistogramVec
	WorkspaceUpdatesErrorsTotal    prometheus.CounterVec
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		WorkspaceUpdatesLatencySeconds: *prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "workspace_updates_latency_seconds",
			Help:      "Time between starting a workspace build and receiving both the agent update and workspace update",
		}, []string{"username", "num_owned_workspaces", "workspace_name"}),
		WorkspaceUpdatesErrorsTotal: *prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Subsystem: "scaletest",
			Name:      "workspace_updates_errors_total",
			Help:      "Total number of workspace updates errors",
		}, []string{"username", "num_owned_workspaces", "action"}),
	}

	reg.MustRegister(m.WorkspaceUpdatesLatencySeconds)
	reg.MustRegister(m.WorkspaceUpdatesErrorsTotal)
	return m
}

func (m *Metrics) RecordCompletion(elapsed time.Duration, username string, ownedWorkspaces int64, workspace string) {
	m.WorkspaceUpdatesLatencySeconds.WithLabelValues(username, strconv.Itoa(int(ownedWorkspaces)), workspace).Observe(elapsed.Seconds())
}

func (m *Metrics) AddError(username string, ownedWorkspaces int64, action string) {
	m.WorkspaceUpdatesErrorsTotal.WithLabelValues(username, strconv.Itoa(int(ownedWorkspaces)), action).Inc()
}
