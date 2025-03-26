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
	createdPrebuildsDesc  = prometheus.NewDesc("coderd_prebuilds_created", "The number of prebuilds that have been created to meet the desired count set by presets.", []string{"template_name", "preset_name"}, nil)
	failedPrebuildsDesc   = prometheus.NewDesc("coderd_prebuilds_failed", "The number of prebuilds that failed to build during creation.", []string{"template_name", "preset_name"}, nil)
	claimedPrebuildsDesc  = prometheus.NewDesc("coderd_prebuilds_claimed", "The number of prebuilds that were claimed by a user. Each count means that a user created a workspace using a preset and was assigned a prebuild instead of a brand new workspace.", []string{"template_name", "preset_name"}, nil)
	usedPresetsDesc       = prometheus.NewDesc("coderd_prebuilds_used_presets", "The number of times a preset was used to build a prebuild.", []string{"template_name", "preset_name"}, nil)
	desiredPrebuildsDesc  = prometheus.NewDesc("coderd_prebuilds_desired", "The number of prebuilds desired by each preset of each template.", []string{"template_name", "preset_name"}, nil)
	runningPrebuildsDesc  = prometheus.NewDesc("coderd_prebuilds_running", "The number of prebuilds that are currently running. Running prebuilds have successfully started, but they may not be ready to be claimed by a user yet.", []string{"template_name", "preset_name"}, nil)
	eligiblePrebuildsDesc = prometheus.NewDesc("coderd_prebuilds_eligible", "The number of eligible prebuilds. Eligible prebuilds are prebuilds that are ready to be claimed by a user.", []string{"template_name", "preset_name"}, nil)
)

type MetricsCollector struct {
	database   database.Store
	logger     slog.Logger
	reconciler prebuilds.Reconciler
}

var _ prometheus.Collector = new(MetricsCollector)

func NewMetricsCollector(db database.Store, logger slog.Logger, reconciler prebuilds.Reconciler) *MetricsCollector {
	return &MetricsCollector{
		database:   db,
		logger:     logger.Named("prebuilds_metrics_collector"),
		reconciler: reconciler,
	}
}

func (*MetricsCollector) Describe(descCh chan<- *prometheus.Desc) {
	descCh <- createdPrebuildsDesc
	descCh <- failedPrebuildsDesc
	descCh <- claimedPrebuildsDesc
	descCh <- usedPresetsDesc
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
		metricsCh <- prometheus.MustNewConstMetric(createdPrebuildsDesc, prometheus.CounterValue, float64(metric.CreatedCount), metric.TemplateName, metric.PresetName)
		metricsCh <- prometheus.MustNewConstMetric(failedPrebuildsDesc, prometheus.CounterValue, float64(metric.FailedCount), metric.TemplateName, metric.PresetName)
		metricsCh <- prometheus.MustNewConstMetric(claimedPrebuildsDesc, prometheus.CounterValue, float64(metric.ClaimedCount), metric.TemplateName, metric.PresetName)
	}

	state, err := mc.reconciler.SnapshotState(ctx, mc.database)
	if err != nil {
		mc.logger.Error(ctx, "failed to get latest prebuild state", slog.Error(err))
		return
	}

	for _, preset := range state.Presets {
		if !preset.UsingActiveVersion {
			continue
		}

		presetState, err := state.FilterByPreset(preset.ID)
		if err != nil {
			mc.logger.Error(ctx, "failed to filter by preset", slog.Error(err))
			continue
		}
		actions, err := mc.reconciler.DetermineActions(ctx, *presetState)
		if err != nil {
			mc.logger.Error(ctx, "failed to determine actions", slog.Error(err))
			continue
		}

		metricsCh <- prometheus.MustNewConstMetric(desiredPrebuildsDesc, prometheus.GaugeValue, float64(actions.Desired), preset.TemplateName, preset.Name)
		metricsCh <- prometheus.MustNewConstMetric(runningPrebuildsDesc, prometheus.GaugeValue, float64(actions.Actual), preset.TemplateName, preset.Name)
		metricsCh <- prometheus.MustNewConstMetric(eligiblePrebuildsDesc, prometheus.GaugeValue, float64(actions.Eligible), preset.TemplateName, preset.Name)
	}
}
