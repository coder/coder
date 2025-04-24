package prebuilds

import (
	"context"
	"time"

	"cdr.dev/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/prebuilds"
)

var (
	labels               = []string{"template_name", "preset_name", "organization_name"}
	createdPrebuildsDesc = prometheus.NewDesc(
		"coderd_prebuilt_workspaces_created_total",
		"The number of prebuilt workspaces that have been created to meet the desired count set by presets.",
		labels,
		nil,
	)
	failedPrebuildsDesc = prometheus.NewDesc(
		"coderd_prebuilt_workspaces_failed_total",
		"The number of prebuilt workspaces that failed to build.",
		labels,
		nil,
	)
	claimedPrebuildsDesc = prometheus.NewDesc(
		"coderd_prebuilt_workspaces_claimed_total",
		"The number of prebuilt workspaces that were claimed by a user. Each count means that a user created a workspace using a preset and claimed a prebuilt workspace instead of a brand new workspace being created.",
		labels,
		nil,
	)
	desiredPrebuildsDesc = prometheus.NewDesc(
		"coderd_prebuilt_workspaces_desired",
		"The number of prebuilt workspaces desired by each preset of each template.",
		labels,
		nil,
	)
	runningPrebuildsDesc = prometheus.NewDesc(
		"coderd_prebuilt_workspaces_running",
		"The number of prebuilt workspaces that are currently running. Running prebuilt workspaces have successfully started, but includes both eligible and ineligible workspaces.",
		labels,
		nil,
	)
	eligiblePrebuildsDesc = prometheus.NewDesc(
		"coderd_prebuilt_workspaces_eligible",
		"The number of eligible prebuilt workspaces. Eligible prebuilt workspaces are ones whose agent is marked 'ready', and can be claimed by a user.",
		labels,
		nil,
	)
)

type MetricsCollector struct {
	database    database.Store
	logger      slog.Logger
	snapshotter prebuilds.StateSnapshotter
}

var _ prometheus.Collector = new(MetricsCollector)

func NewMetricsCollector(db database.Store, logger slog.Logger, snapshotter prebuilds.StateSnapshotter) *MetricsCollector {
	return &MetricsCollector{
		database:    db,
		logger:      logger.Named("prebuilds_metrics_collector"),
		snapshotter: snapshotter,
	}
}

func (*MetricsCollector) Describe(descCh chan<- *prometheus.Desc) {
	descCh <- createdPrebuildsDesc
	descCh <- failedPrebuildsDesc
	descCh <- claimedPrebuildsDesc
	descCh <- desiredPrebuildsDesc
	descCh <- runningPrebuildsDesc
	descCh <- eligiblePrebuildsDesc
}

func (mc *MetricsCollector) Collect(metricsCh chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(dbauthz.AsPrebuildsOrchestrator(context.Background()), 10*time.Second)
	defer cancel()
	// nolint:gocritic // just until we get back to this
	prebuildMetrics, err := mc.database.GetPrebuildMetrics(ctx)
	if err != nil {
		mc.logger.Error(ctx, "failed to get prebuild metrics", slog.Error(err))
		return
	}

	for _, metric := range prebuildMetrics {
		metricsCh <- prometheus.MustNewConstMetric(createdPrebuildsDesc, prometheus.CounterValue, float64(metric.CreatedCount), metric.TemplateName, metric.PresetName, metric.OrganizationName)
		metricsCh <- prometheus.MustNewConstMetric(failedPrebuildsDesc, prometheus.CounterValue, float64(metric.FailedCount), metric.TemplateName, metric.PresetName, metric.OrganizationName)
		metricsCh <- prometheus.MustNewConstMetric(claimedPrebuildsDesc, prometheus.CounterValue, float64(metric.ClaimedCount), metric.TemplateName, metric.PresetName, metric.OrganizationName)
	}

	snapshot, err := mc.snapshotter.SnapshotState(ctx, mc.database)
	if err != nil {
		mc.logger.Error(ctx, "failed to get latest prebuild state", slog.Error(err))
		return
	}

	for _, preset := range snapshot.Presets {
		if !preset.UsingActiveVersion {
			continue
		}

		presetSnapshot, err := snapshot.FilterByPreset(preset.ID)
		if err != nil {
			mc.logger.Error(ctx, "failed to filter by preset", slog.Error(err))
			continue
		}
		state := presetSnapshot.CalculateState()

		metricsCh <- prometheus.MustNewConstMetric(desiredPrebuildsDesc, prometheus.GaugeValue, float64(state.Desired), preset.TemplateName, preset.Name, preset.OrganizationName)
		metricsCh <- prometheus.MustNewConstMetric(runningPrebuildsDesc, prometheus.GaugeValue, float64(state.Actual), preset.TemplateName, preset.Name, preset.OrganizationName)
		metricsCh <- prometheus.MustNewConstMetric(eligiblePrebuildsDesc, prometheus.GaugeValue, float64(state.Eligible), preset.TemplateName, preset.Name, preset.OrganizationName)
	}
}
