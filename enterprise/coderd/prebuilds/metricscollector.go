package prebuilds

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"cdr.dev/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/prebuilds"
)

const (
	namespace = "coderd_prebuilt_workspaces_"

	MetricCreatedCount              = namespace + "created_total"
	MetricFailedCount               = namespace + "failed_total"
	MetricClaimedCount              = namespace + "claimed_total"
	MetricResourceReplacementsCount = namespace + "resource_replacements_total"
	MetricDesiredGauge              = namespace + "desired"
	MetricRunningGauge              = namespace + "running"
	MetricEligibleGauge             = namespace + "eligible"
)

var (
	labels               = []string{"template_name", "preset_name", "organization_name"}
	createdPrebuildsDesc = prometheus.NewDesc(
		MetricCreatedCount,
		"Total number of prebuilt workspaces that have been created to meet the desired instance count of each "+
			"template preset.",
		labels,
		nil,
	)
	failedPrebuildsDesc = prometheus.NewDesc(
		MetricFailedCount,
		"Total number of prebuilt workspaces that failed to build.",
		labels,
		nil,
	)
	claimedPrebuildsDesc = prometheus.NewDesc(
		MetricClaimedCount,
		"Total number of prebuilt workspaces which were claimed by users. Claiming refers to creating a workspace "+
			"with a preset selected for which eligible prebuilt workspaces are available and one is reassigned to a user.",
		labels,
		nil,
	)
	resourceReplacementsDesc = prometheus.NewDesc(
		MetricResourceReplacementsCount,
		"Total number of prebuilt workspaces whose resource(s) got replaced upon being claimed. "+
			"In Terraform, drift on immutable attributes results in resource replacement. "+
			"This represents a worst-case scenario for prebuilt workspaces because the pre-provisioned resource "+
			"would have been recreated when claiming, thus obviating the point of pre-provisioning. "+
			"See https://coder.com/docs/admin/templates/extending-templates/prebuilt-workspaces.md#preventing-resource-replacement",
		labels,
		nil,
	)
	desiredPrebuildsDesc = prometheus.NewDesc(
		MetricDesiredGauge,
		"Target number of prebuilt workspaces that should be available for each template preset.",
		labels,
		nil,
	)
	runningPrebuildsDesc = prometheus.NewDesc(
		MetricRunningGauge,
		"Current number of prebuilt workspaces that are in a running state. These workspaces have started "+
			"successfully but may not yet be claimable by users (see coderd_prebuilt_workspaces_eligible).",
		labels,
		nil,
	)
	eligiblePrebuildsDesc = prometheus.NewDesc(
		MetricEligibleGauge,
		"Current number of prebuilt workspaces that are eligible to be claimed by users. These are workspaces that "+
			"have completed their build process with their agent reporting 'ready' status.",
		labels,
		nil,
	)
)

type MetricsCollector struct {
	database    database.Store
	logger      slog.Logger
	snapshotter prebuilds.StateSnapshotter

	replacementsCounter   map[replacementKey]*atomic.Int64
	replacementsCounterMu sync.Mutex
}

var _ prometheus.Collector = new(MetricsCollector)

func NewMetricsCollector(db database.Store, logger slog.Logger, snapshotter prebuilds.StateSnapshotter) *MetricsCollector {
	return &MetricsCollector{
		database:            db,
		logger:              logger.Named("prebuilds_metrics_collector"),
		snapshotter:         snapshotter,
		replacementsCounter: make(map[replacementKey]*atomic.Int64),
	}
}

func (*MetricsCollector) Describe(descCh chan<- *prometheus.Desc) {
	descCh <- createdPrebuildsDesc
	descCh <- failedPrebuildsDesc
	descCh <- claimedPrebuildsDesc
	descCh <- resourceReplacementsDesc
	descCh <- desiredPrebuildsDesc
	descCh <- runningPrebuildsDesc
	descCh <- eligiblePrebuildsDesc
}

func (mc *MetricsCollector) Collect(metricsCh chan<- prometheus.Metric) {
	// nolint:gocritic // We need to set an authz context to read metrics from the db.
	ctx, cancel := context.WithTimeout(dbauthz.AsPrebuildsOrchestrator(context.Background()), 10*time.Second)
	defer cancel()
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

	mc.replacementsCounterMu.Lock()
	for key, val := range mc.replacementsCounter {
		metricsCh <- prometheus.MustNewConstMetric(resourceReplacementsDesc, prometheus.CounterValue, float64(val.Load()), key.templateName, key.presetName, key.orgName)
	}
	mc.replacementsCounterMu.Unlock()

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

type replacementKey struct {
	orgName, templateName, presetName string
}

func (k replacementKey) String() string {
	return fmt.Sprintf("%s:%s:%s", k.orgName, k.templateName, k.presetName)
}

func (mc *MetricsCollector) trackResourceReplacement(orgName, templateName, presetName string) {
	mc.replacementsCounterMu.Lock()
	defer mc.replacementsCounterMu.Unlock()

	key := replacementKey{orgName: orgName, templateName: templateName, presetName: presetName}
	if _, ok := mc.replacementsCounter[key]; !ok {
		mc.replacementsCounter[key] = &atomic.Int64{}
	}

	// We only track _that_ a resource replacement occurred, not how many.
	// Just one is enough to ruin a prebuild, but we can't know apriori which replacement would cause this.
	// For example, say we have 2 replacements: a docker_container and a null_resource; we don't know which one might
	// cause an issue (or indeed if either would), so we just track the replacement.
	mc.replacementsCounter[key].Add(1)
}
