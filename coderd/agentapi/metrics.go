package agentapi

import (
	"context"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog/v3"

	"github.com/google/uuid"
)

// BuildDurationMetricName is the short name for the end-to-end
// workspace build duration histogram. The full metric name is
// prefixed with the namespace "coderd_".
const BuildDurationMetricName = "template_workspace_build_duration_seconds"

// LifecycleMetrics contains Prometheus metrics for the lifecycle API.
type LifecycleMetrics struct {
	BuildDuration *prometheus.HistogramVec
}

// NewLifecycleMetrics creates and registers all lifecycle-related
// Prometheus metrics.
//
// The build duration histogram tracks the end-to-end duration from
// workspace build creation to agent ready, by template. It is
// recorded by the coderd replica handling the agent's connection
// when the last agent reports ready. In multi-replica deployments,
// each replica only has observations for agents it handles.
//
// The "is_prebuild" label distinguishes prebuild creation (background,
// no user waiting) from user-initiated builds (regular workspace
// creation or prebuild claims).
func NewLifecycleMetrics(reg prometheus.Registerer) *LifecycleMetrics {
	m := &LifecycleMetrics{
		BuildDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
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
		}, []string{"template_name", "organization_name", "transition", "status", "is_prebuild"}),
	}
	reg.MustRegister(m.BuildDuration)
	return m
}

// emitBuildDurationMetric records the end-to-end workspace build
// duration from build creation to when all agents are ready.
func (a *LifecycleAPI) emitBuildDurationMetric(ctx context.Context, resourceID uuid.UUID) {
	if a.Metrics == nil {
		return
	}

	buildInfo, err := a.Database.GetWorkspaceBuildMetricsByResourceID(ctx, resourceID)
	if err != nil {
		a.Log.Warn(ctx, "failed to get build info for metrics", slog.Error(err))
		return
	}

	// Wait until all agents have reached a terminal startup state.
	if !buildInfo.AllAgentsReady {
		return
	}

	// LastAgentReadyAt is the MAX(ready_at) across all agents. Since
	// we only get here when AllAgentsReady is true, this should always
	// be valid.
	if buildInfo.LastAgentReadyAt.IsZero() {
		a.Log.Warn(ctx, "last_agent_ready_at is unexpectedly zero",
			slog.F("last_agent_ready_at", buildInfo.LastAgentReadyAt))
		return
	}

	duration := buildInfo.LastAgentReadyAt.Sub(buildInfo.CreatedAt).Seconds()

	a.Metrics.BuildDuration.WithLabelValues(
		buildInfo.TemplateName,
		buildInfo.OrganizationName,
		string(buildInfo.Transition),
		buildInfo.WorstStatus,
		strconv.FormatBool(buildInfo.IsPrebuild),
	).Observe(duration)
}
