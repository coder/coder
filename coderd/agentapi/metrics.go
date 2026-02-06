package agentapi

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
)

// WorkspaceBuildDurationSeconds is a histogram that tracks the end-to-end
// duration from workspace build creation to agent ready, by template.
//
// This metric is recorded by the coderd replica handling the agent's
// connection when the last agent reports ready. In multi-replica deployments,
// each replica only has observations for agents it handles. Prometheus
// should be configured to scrape all replicas, and queries should aggregate
// across instances:
//
//	histogram_quantile(0.95,
//	  sum(rate(coderd_template_workspace_build_duration_seconds_bucket[5m])) by (le, template_name)
//	)
//
// BuildDurationMetricName is the short name for the end-to-end
// workspace build duration histogram. The full metric name is
// prefixed with the namespace "coderd_".
const BuildDurationMetricName = "template_workspace_build_duration_seconds"

// The "prebuild" label distinguishes prebuild creation (background, no user
// waiting) from user-initiated builds (regular workspace creation or prebuild
// claims).
var WorkspaceBuildDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: "coderd",
	Name:      BuildDurationMetricName,
	Help:      "Duration from workspace build creation to agent ready, by template.",
	Buckets: []float64{
		1, // 1s
		10,
		30,
		60, // 1min
		60 * 5,
		60 * 10,
		60 * 30, // 30min
		60 * 60, // 1hr
	},
	NativeHistogramBucketFactor:     1.1,
	NativeHistogramMaxBucketNumber:  100,
	NativeHistogramMinResetDuration: time.Hour,
}, []string{"template_name", "organization_name", "transition", "status", "is_prebuild"})

// emitBuildDurationMetric records the end-to-end workspace build duration
// from build creation to when all agents are ready.
func emitBuildDurationMetric(
	ctx context.Context,
	log slog.Logger,
	db database.Store,
	histogram *prometheus.HistogramVec,
	resourceID uuid.UUID,
) {
	buildInfo, err := db.GetWorkspaceBuildMetricsByResourceID(ctx, resourceID)
	if err != nil {
		log.Warn(ctx, "failed to get build info for metrics", slog.Error(err))
		return
	}

	// Wait until all agents have reached a terminal startup state.
	if !buildInfo.AllAgentsReady {
		return
	}

	// LastAgentReadyAt is the MAX(ready_at) across all agents. Since we only
	// get here when AllAgentsReady is true, this should always be valid.
	if buildInfo.LastAgentReadyAt.IsZero() {
		log.Warn(ctx, "last_agent_ready_at is unexpectedly zero",
			slog.F("last_agent_ready_at", buildInfo.LastAgentReadyAt))
		return
	}

	duration := buildInfo.LastAgentReadyAt.Sub(buildInfo.CreatedAt).Seconds()

	prebuild := "false"
	if buildInfo.IsPrebuild {
		prebuild = "true"
	}

	histogram.WithLabelValues(
		buildInfo.TemplateName,
		buildInfo.OrganizationName,
		string(buildInfo.Transition),
		buildInfo.WorstStatus,
		prebuild,
	).Observe(duration)
}
