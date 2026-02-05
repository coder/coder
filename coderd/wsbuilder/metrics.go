package wsbuilder

import "github.com/prometheus/client_golang/prometheus"

// Metrics holds metrics related to workspace build creation.
type Metrics struct {
	workspaceBuildsEnqueued *prometheus.CounterVec
}

// Metric label values for build status.
const (
	BuildStatusSuccess = "success"
	BuildStatusFailed  = "failed"
)

// BuildReasonPrebuild is the build_reason metric label value for prebuild
// operations. This is distinct from database.BuildReason values since prebuilds
// use BuildReasonInitiator in the database but we want to track them separately
// in metrics.
const BuildReasonPrebuild = "prebuild"

func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
	m := &Metrics{
		workspaceBuildsEnqueued: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "coderd",
			Name:      "workspace_builds_enqueued_total",
			Help:      "Total number of workspace build enqueue attempts.",
		}, []string{"provisioner_type", "build_reason", "transition", "status"}),
	}

	if reg != nil {
		if err := reg.Register(m.workspaceBuildsEnqueued); err != nil {
			return nil, err
		}
	}

	return m, nil
}

// RecordBuildEnqueued records a workspace build enqueue attempt. It determines
// the status based on whether an error occurred and increments the counter.
func (m *Metrics) RecordBuildEnqueued(provisionerType, buildReason, transition string, err error) {
	status := BuildStatusSuccess
	if err != nil {
		status = BuildStatusFailed
	}
	m.workspaceBuildsEnqueued.WithLabelValues(provisionerType, buildReason, transition, status).Inc()
}
