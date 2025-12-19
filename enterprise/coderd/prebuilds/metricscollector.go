package prebuilds

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
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
	MetricPresetHardLimitedGauge    = namespace + "preset_hard_limited"
	MetricLastUpdatedGauge          = namespace + "metrics_last_updated"
	MetricReconciliationPausedGauge = namespace + "reconciliation_paused"
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
			"See https://coder.com/docs/admin/templates/extending-templates/prebuilt-workspaces#preventing-resource-replacement",
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
	presetHardLimitedDesc = prometheus.NewDesc(
		MetricPresetHardLimitedGauge,
		"Indicates whether a given preset has reached the hard failure limit (1 = hard-limited). Metric is omitted otherwise.",
		labels,
		nil,
	)
	lastUpdateDesc = prometheus.NewDesc(
		MetricLastUpdatedGauge,
		"The unix timestamp when the metrics related to prebuilt workspaces were last updated; these metrics are cached.",
		[]string{},
		nil,
	)
	reconciliationPausedDesc = prometheus.NewDesc(
		MetricReconciliationPausedGauge,
		"Indicates whether prebuilds reconciliation is currently paused (1 = paused, 0 = not paused).",
		[]string{},
		nil,
	)
)

const (
	metricsUpdateInterval = time.Second * 60
	metricsUpdateTimeout  = time.Second * 10
)

type MetricsCollector struct {
	database    database.Store
	logger      slog.Logger
	snapshotter prebuilds.StateSnapshotter

	latestState atomic.Pointer[metricsState]

	replacementsCounter   map[replacementKey]float64
	replacementsCounterMu sync.Mutex

	isPresetHardLimited   map[hardLimitedPresetKey]bool
	isPresetHardLimitedMu sync.Mutex

	reconciliationPaused   bool
	reconciliationPausedMu sync.RWMutex
}

var _ prometheus.Collector = new(MetricsCollector)

func NewMetricsCollector(db database.Store, logger slog.Logger, snapshotter prebuilds.StateSnapshotter) *MetricsCollector {
	log := logger.Named("prebuilds_metrics_collector")

	return &MetricsCollector{
		database:            db,
		logger:              log,
		snapshotter:         snapshotter,
		replacementsCounter: make(map[replacementKey]float64),
		isPresetHardLimited: make(map[hardLimitedPresetKey]bool),
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
	descCh <- presetHardLimitedDesc
	descCh <- lastUpdateDesc
	descCh <- reconciliationPausedDesc
}

// Collect uses the cached state to set configured metrics.
// The state is cached because this function can be called multiple times per second and retrieving the current state
// is an expensive operation.
func (mc *MetricsCollector) Collect(metricsCh chan<- prometheus.Metric) {
	mc.reconciliationPausedMu.RLock()
	var pausedValue float64
	if mc.reconciliationPaused {
		pausedValue = 1
	}
	mc.reconciliationPausedMu.RUnlock()

	metricsCh <- prometheus.MustNewConstMetric(reconciliationPausedDesc, prometheus.GaugeValue, pausedValue)

	currentState := mc.latestState.Load() // Grab a copy; it's ok if it goes stale during the course of this func.
	if currentState == nil {
		mc.logger.Warn(context.Background(), "failed to set prebuilds metrics; state not set")
		metricsCh <- prometheus.MustNewConstMetric(lastUpdateDesc, prometheus.GaugeValue, 0)
		return
	}

	for _, metric := range currentState.prebuildMetrics {
		metricsCh <- prometheus.MustNewConstMetric(createdPrebuildsDesc, prometheus.CounterValue, float64(metric.CreatedCount), metric.TemplateName, metric.PresetName, metric.OrganizationName)
		metricsCh <- prometheus.MustNewConstMetric(failedPrebuildsDesc, prometheus.CounterValue, float64(metric.FailedCount), metric.TemplateName, metric.PresetName, metric.OrganizationName)
		metricsCh <- prometheus.MustNewConstMetric(claimedPrebuildsDesc, prometheus.CounterValue, float64(metric.ClaimedCount), metric.TemplateName, metric.PresetName, metric.OrganizationName)
	}

	mc.replacementsCounterMu.Lock()
	for key, val := range mc.replacementsCounter {
		metricsCh <- prometheus.MustNewConstMetric(resourceReplacementsDesc, prometheus.CounterValue, val, key.templateName, key.presetName, key.orgName)
	}
	mc.replacementsCounterMu.Unlock()

	for _, preset := range currentState.snapshot.Presets {
		if !preset.UsingActiveVersion {
			continue
		}

		if preset.Deleted {
			continue
		}

		presetSnapshot, err := currentState.snapshot.FilterByPreset(preset.ID)
		if err != nil {
			mc.logger.Error(context.Background(), "failed to filter by preset", slog.Error(err))
			continue
		}
		state := presetSnapshot.CalculateState()

		metricsCh <- prometheus.MustNewConstMetric(desiredPrebuildsDesc, prometheus.GaugeValue, float64(state.Desired), preset.TemplateName, preset.Name, preset.OrganizationName)
		metricsCh <- prometheus.MustNewConstMetric(runningPrebuildsDesc, prometheus.GaugeValue, float64(state.Actual), preset.TemplateName, preset.Name, preset.OrganizationName)
		metricsCh <- prometheus.MustNewConstMetric(eligiblePrebuildsDesc, prometheus.GaugeValue, float64(state.Eligible), preset.TemplateName, preset.Name, preset.OrganizationName)
	}

	mc.isPresetHardLimitedMu.Lock()
	for key, isHardLimited := range mc.isPresetHardLimited {
		var val float64
		if isHardLimited {
			val = 1
		}

		metricsCh <- prometheus.MustNewConstMetric(presetHardLimitedDesc, prometheus.GaugeValue, val, key.templateName, key.presetName, key.orgName)
	}
	mc.isPresetHardLimitedMu.Unlock()

	metricsCh <- prometheus.MustNewConstMetric(lastUpdateDesc, prometheus.GaugeValue, float64(currentState.createdAt.Unix()))
}

type metricsState struct {
	prebuildMetrics []database.GetPrebuildMetricsRow
	snapshot        *prebuilds.GlobalSnapshot
	createdAt       time.Time
}

// BackgroundFetch updates the metrics state every given interval.
func (mc *MetricsCollector) BackgroundFetch(ctx context.Context, updateInterval, updateTimeout time.Duration) {
	tick := time.NewTicker(time.Nanosecond)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			// Tick immediately, then set regular interval.
			tick.Reset(updateInterval)

			if err := mc.UpdateState(ctx, updateTimeout); err != nil {
				mc.logger.Error(ctx, "failed to update prebuilds metrics state", slog.Error(err))
			}
		}
	}
}

// UpdateState builds the current metrics state.
func (mc *MetricsCollector) UpdateState(ctx context.Context, timeout time.Duration) error {
	start := time.Now()
	fetchCtx, fetchCancel := context.WithTimeout(ctx, timeout)
	defer fetchCancel()

	prebuildMetrics, err := mc.database.GetPrebuildMetrics(fetchCtx)
	if err != nil {
		return xerrors.Errorf("fetch prebuild metrics: %w", err)
	}

	snapshot, err := mc.snapshotter.SnapshotState(fetchCtx, mc.database)
	if err != nil {
		return xerrors.Errorf("snapshot state: %w", err)
	}
	mc.logger.Debug(ctx, "fetched prebuilds metrics state", slog.F("duration_secs", fmt.Sprintf("%.2f", time.Since(start).Seconds())))

	mc.latestState.Store(&metricsState{
		prebuildMetrics: prebuildMetrics,
		snapshot:        snapshot,
		createdAt:       dbtime.Now(),
	})
	return nil
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

	// We only track _that_ a resource replacement occurred, not how many.
	// Just one is enough to ruin a prebuild, but we can't know apriori which replacement would cause this.
	// For example, say we have 2 replacements: a docker_container and a null_resource; we don't know which one might
	// cause an issue (or indeed if either would), so we just track the replacement.
	mc.replacementsCounter[key]++
}

type hardLimitedPresetKey struct {
	orgName, templateName, presetName string
}

func (k hardLimitedPresetKey) String() string {
	return fmt.Sprintf("%s:%s:%s", k.orgName, k.templateName, k.presetName)
}

func (mc *MetricsCollector) registerHardLimitedPresets(isPresetHardLimited map[hardLimitedPresetKey]bool) {
	mc.isPresetHardLimitedMu.Lock()
	defer mc.isPresetHardLimitedMu.Unlock()

	mc.isPresetHardLimited = isPresetHardLimited
}

func (mc *MetricsCollector) setReconciliationPaused(paused bool) {
	mc.reconciliationPausedMu.Lock()
	defer mc.reconciliationPausedMu.Unlock()

	mc.reconciliationPaused = paused
}
