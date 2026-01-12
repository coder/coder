package prebuilds_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	prometheus_client "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"tailscale.com/types/ptr"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/files"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestMetricsCollector(t *testing.T) {
	t.Parallel()

	type metricCheck struct {
		name      string
		value     *float64
		isCounter bool
	}

	type testCase struct {
		name            string
		transitions     []database.WorkspaceTransition
		jobStatuses     []database.ProvisionerJobStatus
		initiatorIDs    []uuid.UUID
		ownerIDs        []uuid.UUID
		metrics         []metricCheck
		templateDeleted []bool
		eligible        []bool
	}

	tests := []testCase{
		{
			name:         "prebuild provisioned but not completed",
			transitions:  allTransitions,
			jobStatuses:  allJobStatusesExcept(database.ProvisionerJobStatusPending, database.ProvisionerJobStatusRunning, database.ProvisionerJobStatusCanceling),
			initiatorIDs: []uuid.UUID{database.PrebuildsSystemUserID},
			ownerIDs:     []uuid.UUID{database.PrebuildsSystemUserID},
			metrics: []metricCheck{
				{prebuilds.MetricCreatedCount, ptr.To(1.0), true},
				{prebuilds.MetricClaimedCount, ptr.To(0.0), true},
				{prebuilds.MetricFailedCount, ptr.To(0.0), true},
				{prebuilds.MetricDesiredGauge, ptr.To(1.0), false},
				{prebuilds.MetricRunningGauge, ptr.To(0.0), false},
				{prebuilds.MetricEligibleGauge, ptr.To(0.0), false},
			},
			templateDeleted: []bool{false},
			eligible:        []bool{false},
		},
		{
			name:         "prebuild running",
			transitions:  []database.WorkspaceTransition{database.WorkspaceTransitionStart},
			jobStatuses:  []database.ProvisionerJobStatus{database.ProvisionerJobStatusSucceeded},
			initiatorIDs: []uuid.UUID{database.PrebuildsSystemUserID},
			ownerIDs:     []uuid.UUID{database.PrebuildsSystemUserID},
			metrics: []metricCheck{
				{prebuilds.MetricCreatedCount, ptr.To(1.0), true},
				{prebuilds.MetricClaimedCount, ptr.To(0.0), true},
				{prebuilds.MetricFailedCount, ptr.To(0.0), true},
				{prebuilds.MetricDesiredGauge, ptr.To(1.0), false},
				{prebuilds.MetricRunningGauge, ptr.To(1.0), false},
				{prebuilds.MetricEligibleGauge, ptr.To(0.0), false},
			},
			templateDeleted: []bool{false},
			eligible:        []bool{false},
		},
		{
			name:         "prebuild failed",
			transitions:  allTransitions,
			jobStatuses:  []database.ProvisionerJobStatus{database.ProvisionerJobStatusFailed},
			initiatorIDs: []uuid.UUID{database.PrebuildsSystemUserID},
			ownerIDs:     []uuid.UUID{database.PrebuildsSystemUserID, uuid.New()},
			metrics: []metricCheck{
				{prebuilds.MetricCreatedCount, ptr.To(1.0), true},
				{prebuilds.MetricFailedCount, ptr.To(1.0), true},
				{prebuilds.MetricDesiredGauge, ptr.To(1.0), false},
				{prebuilds.MetricRunningGauge, ptr.To(0.0), false},
				{prebuilds.MetricEligibleGauge, ptr.To(0.0), false},
			},
			templateDeleted: []bool{false},
			eligible:        []bool{false},
		},
		{
			name:         "prebuild eligible",
			transitions:  []database.WorkspaceTransition{database.WorkspaceTransitionStart},
			jobStatuses:  []database.ProvisionerJobStatus{database.ProvisionerJobStatusSucceeded},
			initiatorIDs: []uuid.UUID{database.PrebuildsSystemUserID},
			ownerIDs:     []uuid.UUID{database.PrebuildsSystemUserID},
			metrics: []metricCheck{
				{prebuilds.MetricCreatedCount, ptr.To(1.0), true},
				{prebuilds.MetricClaimedCount, ptr.To(0.0), true},
				{prebuilds.MetricFailedCount, ptr.To(0.0), true},
				{prebuilds.MetricDesiredGauge, ptr.To(1.0), false},
				{prebuilds.MetricRunningGauge, ptr.To(1.0), false},
				{prebuilds.MetricEligibleGauge, ptr.To(1.0), false},
			},
			templateDeleted: []bool{false},
			eligible:        []bool{true},
		},
		{
			name:         "prebuild ineligible",
			transitions:  allTransitions,
			jobStatuses:  allJobStatusesExcept(database.ProvisionerJobStatusSucceeded),
			initiatorIDs: []uuid.UUID{database.PrebuildsSystemUserID},
			ownerIDs:     []uuid.UUID{database.PrebuildsSystemUserID},
			metrics: []metricCheck{
				{prebuilds.MetricCreatedCount, ptr.To(1.0), true},
				{prebuilds.MetricClaimedCount, ptr.To(0.0), true},
				{prebuilds.MetricFailedCount, ptr.To(0.0), true},
				{prebuilds.MetricDesiredGauge, ptr.To(1.0), false},
				{prebuilds.MetricRunningGauge, ptr.To(1.0), false},
				{prebuilds.MetricEligibleGauge, ptr.To(0.0), false},
			},
			templateDeleted: []bool{false},
			eligible:        []bool{false},
		},
		{
			name:         "prebuild claimed",
			transitions:  allTransitions,
			jobStatuses:  allJobStatuses,
			initiatorIDs: []uuid.UUID{database.PrebuildsSystemUserID},
			ownerIDs:     []uuid.UUID{uuid.New()},
			metrics: []metricCheck{
				{prebuilds.MetricCreatedCount, ptr.To(1.0), true},
				{prebuilds.MetricClaimedCount, ptr.To(1.0), true},
				{prebuilds.MetricDesiredGauge, ptr.To(1.0), false},
				{prebuilds.MetricRunningGauge, ptr.To(0.0), false},
				{prebuilds.MetricEligibleGauge, ptr.To(0.0), false},
			},
			templateDeleted: []bool{false},
			eligible:        []bool{false},
		},
		{
			name:         "workspaces that were not created by the prebuilds user are not counted",
			transitions:  allTransitions,
			jobStatuses:  allJobStatuses,
			initiatorIDs: []uuid.UUID{uuid.New()},
			ownerIDs:     []uuid.UUID{uuid.New()},
			metrics: []metricCheck{
				{prebuilds.MetricDesiredGauge, ptr.To(1.0), false},
				{prebuilds.MetricRunningGauge, ptr.To(0.0), false},
				{prebuilds.MetricEligibleGauge, ptr.To(0.0), false},
			},
			templateDeleted: []bool{false},
			eligible:        []bool{false},
		},
		{
			name:            "deleted templates should not be included in exported metrics",
			transitions:     allTransitions,
			jobStatuses:     allJobStatuses,
			initiatorIDs:    []uuid.UUID{database.PrebuildsSystemUserID},
			ownerIDs:        []uuid.UUID{database.PrebuildsSystemUserID, uuid.New()},
			metrics:         nil,
			templateDeleted: []bool{true},
			eligible:        []bool{false},
		},
	}
	for _, test := range tests {
		for _, transition := range test.transitions {
			for _, jobStatus := range test.jobStatuses {
				for _, initiatorID := range test.initiatorIDs {
					for _, ownerID := range test.ownerIDs {
						for _, templateDeleted := range test.templateDeleted {
							for _, eligible := range test.eligible {
								t.Run(fmt.Sprintf("%v/transition:%s/jobStatus:%s", test.name, transition, jobStatus), func(t *testing.T) {
									t.Parallel()

									logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
									t.Cleanup(func() {
										if t.Failed() {
											t.Logf("failed to run test: %s", test.name)
											t.Logf("transition: %s", transition)
											t.Logf("jobStatus: %s", jobStatus)
											t.Logf("initiatorID: %s", initiatorID)
											t.Logf("ownerID: %s", ownerID)
											t.Logf("templateDeleted: %t", templateDeleted)
										}
									})
									clock := quartz.NewMock(t)
									db, pubsub := dbtestutil.NewDB(t)
									cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
									reconciler := prebuilds.NewStoreReconciler(db, pubsub, cache, codersdk.PrebuildsConfig{}, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr(), noop.NewTracerProvider())
									ctx := testutil.Context(t, testutil.WaitLong)

									createdUsers := []uuid.UUID{database.PrebuildsSystemUserID}
									for _, user := range slices.Concat(test.ownerIDs, test.initiatorIDs) {
										if !slices.Contains(createdUsers, user) {
											dbgen.User(t, db, database.User{
												ID: user,
											})
											createdUsers = append(createdUsers, user)
										}
									}

									collector := prebuilds.NewMetricsCollector(db, logger, reconciler)
									registry := prometheus.NewPedanticRegistry()
									registry.Register(collector)

									numTemplates := 2
									for i := 0; i < numTemplates; i++ {
										org, template := setupTestDBTemplate(t, db, ownerID, templateDeleted)
										templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, pubsub, org.ID, ownerID, template.ID)
										preset := setupTestDBPreset(t, db, templateVersionID, 1, uuid.New().String())
										workspace, _ := setupTestDBWorkspace(
											t, clock, db, pubsub,
											transition, jobStatus, org.ID, preset, template.ID, templateVersionID, initiatorID, ownerID,
										)
										setupTestDBWorkspaceAgent(t, db, workspace.ID, eligible)
									}

									// Force an update to the metrics state to allow the collector to collect fresh metrics.
									require.NoError(t, collector.UpdateState(dbauthz.AsPrebuildsOrchestrator(ctx), testutil.WaitLong))

									metricsFamilies, err := registry.Gather()
									require.NoError(t, err)

									templates, err := db.GetTemplates(ctx)
									require.NoError(t, err)
									require.Equal(t, numTemplates, len(templates))

									for _, template := range templates {
										org, err := db.GetOrganizationByID(ctx, template.OrganizationID)
										require.NoError(t, err)
										templateVersions, err := db.GetTemplateVersionsByTemplateID(ctx, database.GetTemplateVersionsByTemplateIDParams{
											TemplateID: template.ID,
										})
										require.NoError(t, err)
										require.Equal(t, 1, len(templateVersions))

										presets, err := db.GetPresetsByTemplateVersionID(ctx, templateVersions[0].ID)
										require.NoError(t, err)
										require.Equal(t, 1, len(presets))

										for _, preset := range presets {
											labels := map[string]string{
												"template_name":     template.Name,
												"preset_name":       preset.Name,
												"organization_name": org.Name,
											}

											// If no expected metrics have been defined, ensure we don't find any metric series (i.e. metrics with given labels).
											if test.metrics == nil {
												series := findAllMetricSeries(metricsFamilies, labels)
												require.Empty(t, series)
											}

											for _, check := range test.metrics {
												metric := findMetric(metricsFamilies, check.name, labels)
												if check.value == nil {
													continue
												}

												require.NotNil(t, metric, "metric %s should exist", check.name)

												if check.isCounter {
													require.Equal(t, *check.value, metric.GetCounter().GetValue(), "counter %s value mismatch", check.name)
												} else {
													require.Equal(t, *check.value, metric.GetGauge().GetValue(), "gauge %s value mismatch", check.name)
												}
											}
										}
									}
								})
							}
						}
					}
				}
			}
		}
	}
}

// TestMetricsCollector_DuplicateTemplateNames validates a bug that we saw previously which caused duplicate metric series
// registration when a template was deleted and a new one created with the same name (and preset name).
// We are now excluding deleted templates from our metric collection.
func TestMetricsCollector_DuplicateTemplateNames(t *testing.T) {
	t.Parallel()

	type metricCheck struct {
		name      string
		value     *float64
		isCounter bool
	}

	type testCase struct {
		transition  database.WorkspaceTransition
		jobStatus   database.ProvisionerJobStatus
		initiatorID uuid.UUID
		ownerID     uuid.UUID
		metrics     []metricCheck
		eligible    bool
	}

	test := testCase{
		transition:  database.WorkspaceTransitionStart,
		jobStatus:   database.ProvisionerJobStatusSucceeded,
		initiatorID: database.PrebuildsSystemUserID,
		ownerID:     database.PrebuildsSystemUserID,
		metrics: []metricCheck{
			{prebuilds.MetricCreatedCount, ptr.To(1.0), true},
			{prebuilds.MetricClaimedCount, ptr.To(0.0), true},
			{prebuilds.MetricFailedCount, ptr.To(0.0), true},
			{prebuilds.MetricDesiredGauge, ptr.To(1.0), false},
			{prebuilds.MetricRunningGauge, ptr.To(1.0), false},
			{prebuilds.MetricEligibleGauge, ptr.To(1.0), false},
		},
		eligible: true,
	}

	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	clock := quartz.NewMock(t)
	db, pubsub := dbtestutil.NewDB(t)
	cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
	reconciler := prebuilds.NewStoreReconciler(db, pubsub, cache, codersdk.PrebuildsConfig{}, logger, quartz.NewMock(t), prometheus.NewRegistry(), newNoopEnqueuer(), newNoopUsageCheckerPtr(), noop.NewTracerProvider())
	ctx := testutil.Context(t, testutil.WaitLong)

	collector := prebuilds.NewMetricsCollector(db, logger, reconciler)
	registry := prometheus.NewPedanticRegistry()
	registry.Register(collector)

	presetName := "default-preset"
	defaultOrg := dbgen.Organization(t, db, database.Organization{})
	setupTemplateWithDeps := func() database.Template {
		template := setupTestDBTemplateWithinOrg(t, db, test.ownerID, false, "default-template", defaultOrg)
		templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, pubsub, defaultOrg.ID, test.ownerID, template.ID)
		preset := setupTestDBPreset(t, db, templateVersionID, 1, "default-preset")
		workspace, _ := setupTestDBWorkspace(
			t, clock, db, pubsub,
			test.transition, test.jobStatus, defaultOrg.ID, preset, template.ID, templateVersionID, test.initiatorID, test.ownerID,
		)
		setupTestDBWorkspaceAgent(t, db, workspace.ID, test.eligible)
		return template
	}

	// When: starting with a regular template.
	template := setupTemplateWithDeps()
	labels := map[string]string{
		"template_name":     template.Name,
		"preset_name":       presetName,
		"organization_name": defaultOrg.Name,
	}

	ctx = dbauthz.AsPrebuildsOrchestrator(ctx)

	// Then: metrics collect successfully.
	require.NoError(t, collector.UpdateState(ctx, testutil.WaitLong))
	metricsFamilies, err := registry.Gather()
	require.NoError(t, err)
	require.NotEmpty(t, findAllMetricSeries(metricsFamilies, labels))

	// When: the template is deleted.
	require.NoError(t, db.UpdateTemplateDeletedByID(ctx, database.UpdateTemplateDeletedByIDParams{
		ID:        template.ID,
		Deleted:   true,
		UpdatedAt: dbtime.Now(),
	}))

	// Then: metrics collect successfully but are empty because the template is deleted.
	require.NoError(t, collector.UpdateState(ctx, testutil.WaitLong))
	metricsFamilies, err = registry.Gather()
	require.NoError(t, err)
	require.Empty(t, findAllMetricSeries(metricsFamilies, labels))

	// When: a new template is created with the same name as the deleted template.
	newTemplate := setupTemplateWithDeps()

	// Ensure the database has both the new and old (delete) template.
	{
		deleted, err := db.GetTemplateByOrganizationAndName(ctx, database.GetTemplateByOrganizationAndNameParams{
			OrganizationID: template.OrganizationID,
			Deleted:        true,
			Name:           template.Name,
		})
		require.NoError(t, err)
		require.Equal(t, template.ID, deleted.ID)

		current, err := db.GetTemplateByOrganizationAndName(ctx, database.GetTemplateByOrganizationAndNameParams{
			// Use details from deleted template to ensure they're aligned.
			OrganizationID: template.OrganizationID,
			Deleted:        false,
			Name:           template.Name,
		})
		require.NoError(t, err)
		require.Equal(t, newTemplate.ID, current.ID)
	}

	// Then: metrics collect successfully.
	require.NoError(t, collector.UpdateState(ctx, testutil.WaitLong))
	metricsFamilies, err = registry.Gather()
	require.NoError(t, err)
	require.NotEmpty(t, findAllMetricSeries(metricsFamilies, labels))
}

func findMetric(metricsFamilies []*prometheus_client.MetricFamily, name string, labels map[string]string) *prometheus_client.Metric {
	for _, metricFamily := range metricsFamilies {
		if metricFamily.GetName() != name {
			continue
		}

		for _, metric := range metricFamily.GetMetric() {
			labelPairs := metric.GetLabel()

			// Convert label pairs to map for easier lookup
			metricLabels := make(map[string]string, len(labelPairs))
			for _, label := range labelPairs {
				metricLabels[label.GetName()] = label.GetValue()
			}

			// Check if all requested labels match
			for wantName, wantValue := range labels {
				if metricLabels[wantName] != wantValue {
					continue
				}
			}

			return metric
		}
	}
	return nil
}

// findAllMetricSeries finds all metrics with a given set of labels.
func findAllMetricSeries(metricsFamilies []*prometheus_client.MetricFamily, labels map[string]string) map[string]*prometheus_client.Metric {
	series := make(map[string]*prometheus_client.Metric)
	for _, metricFamily := range metricsFamilies {
		for _, metric := range metricFamily.GetMetric() {
			labelPairs := metric.GetLabel()

			if len(labelPairs) != len(labels) {
				continue
			}

			// Convert label pairs to map for easier lookup
			metricLabels := make(map[string]string, len(labelPairs))
			for _, label := range labelPairs {
				metricLabels[label.GetName()] = label.GetValue()
			}

			// Check if all requested labels match
			for wantName, wantValue := range labels {
				if metricLabels[wantName] != wantValue {
					continue
				}
			}

			series[metricFamily.GetName()] = metric
		}
	}
	return series
}

func TestMetricsCollector_ReconciliationPausedMetric(t *testing.T) {
	t.Parallel()

	t.Run("reconciliation_not_paused", func(t *testing.T) {
		t.Parallel()

		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		db, pubsub := dbtestutil.NewDB(t)
		cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
		registry := prometheus.NewPedanticRegistry()
		reconciler := prebuilds.NewStoreReconciler(db, pubsub, cache, codersdk.PrebuildsConfig{}, logger, quartz.NewMock(t), registry, newNoopEnqueuer(), newNoopUsageCheckerPtr(), noop.NewTracerProvider())
		ctx := testutil.Context(t, testutil.WaitLong)

		// Ensure no pause setting is set (default state)
		err := db.UpsertPrebuildsSettings(ctx, `{}`)
		require.NoError(t, err)

		// Run reconciliation to update the metric
		_, err = reconciler.ReconcileAll(ctx)
		require.NoError(t, err)

		// Check that the metric shows reconciliation is not paused
		metricsFamilies, err := registry.Gather()
		require.NoError(t, err)

		metric := findMetric(metricsFamilies, prebuilds.MetricReconciliationPausedGauge, map[string]string{})
		require.NotNil(t, metric, "reconciliation paused metric should exist")
		require.NotNil(t, metric.GetGauge())
		require.Equal(t, 0.0, metric.GetGauge().GetValue(), "reconciliation should not be paused")
	})

	t.Run("reconciliation_paused", func(t *testing.T) {
		t.Parallel()

		// Create isolated collector and registry for this test
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		db, pubsub := dbtestutil.NewDB(t)
		cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
		registry := prometheus.NewPedanticRegistry()
		reconciler := prebuilds.NewStoreReconciler(db, pubsub, cache, codersdk.PrebuildsConfig{}, logger, quartz.NewMock(t), registry, newNoopEnqueuer(), newNoopUsageCheckerPtr(), noop.NewTracerProvider())
		ctx := testutil.Context(t, testutil.WaitLong)

		// Set reconciliation to paused
		err := prebuilds.SetPrebuildsReconciliationPaused(ctx, db, true)
		require.NoError(t, err)

		// Run reconciliation to update the metric
		_, err = reconciler.ReconcileAll(ctx)
		require.NoError(t, err)

		// Check that the metric shows reconciliation is paused
		metricsFamilies, err := registry.Gather()
		require.NoError(t, err)

		metric := findMetric(metricsFamilies, prebuilds.MetricReconciliationPausedGauge, map[string]string{})
		require.NotNil(t, metric, "reconciliation paused metric should exist")
		require.NotNil(t, metric.GetGauge())
		require.Equal(t, 1.0, metric.GetGauge().GetValue(), "reconciliation should be paused")
	})

	t.Run("reconciliation_resumed", func(t *testing.T) {
		t.Parallel()

		// Create isolated collector and registry for this test
		logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
		db, pubsub := dbtestutil.NewDB(t)
		cache := files.New(prometheus.NewRegistry(), &coderdtest.FakeAuthorizer{})
		registry := prometheus.NewPedanticRegistry()
		reconciler := prebuilds.NewStoreReconciler(db, pubsub, cache, codersdk.PrebuildsConfig{}, logger, quartz.NewMock(t), registry, newNoopEnqueuer(), newNoopUsageCheckerPtr(), noop.NewTracerProvider())
		ctx := testutil.Context(t, testutil.WaitLong)

		// Set reconciliation back to not paused
		err := prebuilds.SetPrebuildsReconciliationPaused(ctx, db, false)
		require.NoError(t, err)

		// Run reconciliation to update the metric
		_, err = reconciler.ReconcileAll(ctx)
		require.NoError(t, err)

		// Check that the metric shows reconciliation is not paused
		metricsFamilies, err := registry.Gather()
		require.NoError(t, err)

		metric := findMetric(metricsFamilies, prebuilds.MetricReconciliationPausedGauge, map[string]string{})
		require.NotNil(t, metric, "reconciliation paused metric should exist")
		require.NotNil(t, metric.GetGauge())
		require.Equal(t, 0.0, metric.GetGauge().GetValue(), "reconciliation should not be paused")
	})
}
