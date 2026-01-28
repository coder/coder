package agentapi

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// WorkspaceBuildDurationSeconds is a histogram that tracks the end-to-end
// duration from workspace build creation to agent ready, by template.
//
// This metric is recorded by the coderd replica handling the agent's
// connection when the agent reports ready. In multi-replica deployments,
// each replica only has observations for agents it handles. Prometheus
// should be configured to scrape all replicas, and queries should aggregate
// across instances:
//
//	histogram_quantile(0.95,
//	  sum(rate(coderd_template_workspace_build_duration_seconds_bucket[5m])) by (le, template_name)
//	)
var WorkspaceBuildDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace:                       "coderd",
	Name:                            "template_workspace_build_duration_seconds",
	Help:                            "Duration from workspace build creation to agent ready, by template.",
	NativeHistogramBucketFactor:     1.1,
	NativeHistogramMaxBucketNumber:  100,
	NativeHistogramMinResetDuration: time.Hour,
}, []string{"template_name", "organization_name", "workspace_transition", "status"})
