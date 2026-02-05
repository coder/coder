package agentapi

import (
	"context"
	"fmt"
	"time"

	"cdr.dev/slog/v3"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

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
// The "prebuild" label distinguishes prebuild creation (background, no user
// waiting) from user-initiated builds (regular workspace creation or prebuild
// claims).
var WorkspaceBuildDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace:                       "coderd",
	Name:                            "template_workspace_build_duration_seconds",
	Help:                            "Duration from workspace build creation to agent ready, by template.",
	NativeHistogramBucketFactor:     1.1,
	NativeHistogramMaxBucketNumber:  100,
	NativeHistogramMinResetDuration: time.Hour,
}, []string{"template_name", "organization_name", "workspace_transition", "status", "prebuild"})

// emitBuildDurationMetric records the end-to-end workspace build duration
// from build creation to when all agents are ready.
func emitBuildDurationMetric(
	ctx context.Context,
	log slog.Logger,
	db database.Store,
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
	lastReadyAt, ok := buildInfo.LastAgentReadyAt.(time.Time)
	if !ok {
		log.Warn(ctx, "unexpected type for last_agent_ready_at",
			slog.F("type", fmt.Sprintf("%T", buildInfo.LastAgentReadyAt)))
		return
	}

	duration := lastReadyAt.Sub(buildInfo.CreatedAt).Seconds()

	prebuild := "false"
	if buildInfo.IsPrebuild {
		prebuild = "true"
	}

	WorkspaceBuildDurationSeconds.WithLabelValues(
		buildInfo.TemplateName,
		buildInfo.OrganizationName,
		string(buildInfo.Transition),
		buildInfo.WorstStatus,
		prebuild,
	).Observe(duration)
}
