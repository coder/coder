package prebuilds_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tailscale.com/types/ptr"

	"github.com/prometheus/client_golang/prometheus"
	prometheus_client "github.com/prometheus/client_model/go"

	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/quartz"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	agplprebuilds "github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/testutil"
)

func TestMetricsCollector(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test requires postgres")
	}

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
			initiatorIDs: []uuid.UUID{agplprebuilds.SystemUserID},
			ownerIDs:     []uuid.UUID{agplprebuilds.SystemUserID},
			metrics: []metricCheck{
				{"coderd_prebuilt_workspaces_created_total", ptr.To(1.0), true},
				{"coderd_prebuilt_workspaces_claimed_total", ptr.To(0.0), true},
				{"coderd_prebuilt_workspaces_failed_total", ptr.To(0.0), true},
				{"coderd_prebuilt_workspaces_desired", ptr.To(1.0), false},
				{"coderd_prebuilt_workspaces_running", ptr.To(0.0), false},
				{"coderd_prebuilt_workspaces_eligible", ptr.To(0.0), false},
			},
			templateDeleted: []bool{false},
			eligible:        []bool{false},
		},
		{
			name:         "prebuild running",
			transitions:  []database.WorkspaceTransition{database.WorkspaceTransitionStart},
			jobStatuses:  []database.ProvisionerJobStatus{database.ProvisionerJobStatusSucceeded},
			initiatorIDs: []uuid.UUID{agplprebuilds.SystemUserID},
			ownerIDs:     []uuid.UUID{agplprebuilds.SystemUserID},
			metrics: []metricCheck{
				{"coderd_prebuilt_workspaces_created_total", ptr.To(1.0), true},
				{"coderd_prebuilt_workspaces_claimed_total", ptr.To(0.0), true},
				{"coderd_prebuilt_workspaces_failed_total", ptr.To(0.0), true},
				{"coderd_prebuilt_workspaces_desired", ptr.To(1.0), false},
				{"coderd_prebuilt_workspaces_running", ptr.To(1.0), false},
				{"coderd_prebuilt_workspaces_eligible", ptr.To(0.0), false},
			},
			templateDeleted: []bool{false},
			eligible:        []bool{false},
		},
		{
			name:         "prebuild failed",
			transitions:  allTransitions,
			jobStatuses:  []database.ProvisionerJobStatus{database.ProvisionerJobStatusFailed},
			initiatorIDs: []uuid.UUID{agplprebuilds.SystemUserID},
			ownerIDs:     []uuid.UUID{agplprebuilds.SystemUserID, uuid.New()},
			metrics: []metricCheck{
				{"coderd_prebuilt_workspaces_created_total", ptr.To(1.0), true},
				{"coderd_prebuilt_workspaces_failed_total", ptr.To(1.0), true},
				{"coderd_prebuilt_workspaces_desired", ptr.To(1.0), false},
				{"coderd_prebuilt_workspaces_running", ptr.To(0.0), false},
				{"coderd_prebuilt_workspaces_eligible", ptr.To(0.0), false},
			},
			templateDeleted: []bool{false},
			eligible:        []bool{false},
		},
		{
			name:         "prebuild eligible",
			transitions:  []database.WorkspaceTransition{database.WorkspaceTransitionStart},
			jobStatuses:  []database.ProvisionerJobStatus{database.ProvisionerJobStatusSucceeded},
			initiatorIDs: []uuid.UUID{agplprebuilds.SystemUserID},
			ownerIDs:     []uuid.UUID{agplprebuilds.SystemUserID},
			metrics: []metricCheck{
				{"coderd_prebuilt_workspaces_created_total", ptr.To(1.0), true},
				{"coderd_prebuilt_workspaces_claimed_total", ptr.To(0.0), true},
				{"coderd_prebuilt_workspaces_failed_total", ptr.To(0.0), true},
				{"coderd_prebuilt_workspaces_desired", ptr.To(1.0), false},
				{"coderd_prebuilt_workspaces_running", ptr.To(1.0), false},
				{"coderd_prebuilt_workspaces_eligible", ptr.To(1.0), false},
			},
			templateDeleted: []bool{false},
			eligible:        []bool{true},
		},
		{
			name:         "prebuild ineligible",
			transitions:  allTransitions,
			jobStatuses:  allJobStatusesExcept(database.ProvisionerJobStatusSucceeded),
			initiatorIDs: []uuid.UUID{agplprebuilds.SystemUserID},
			ownerIDs:     []uuid.UUID{agplprebuilds.SystemUserID},
			metrics: []metricCheck{
				{"coderd_prebuilt_workspaces_created_total", ptr.To(1.0), true},
				{"coderd_prebuilt_workspaces_claimed_total", ptr.To(0.0), true},
				{"coderd_prebuilt_workspaces_failed_total", ptr.To(0.0), true},
				{"coderd_prebuilt_workspaces_desired", ptr.To(1.0), false},
				{"coderd_prebuilt_workspaces_running", ptr.To(1.0), false},
				{"coderd_prebuilt_workspaces_eligible", ptr.To(0.0), false},
			},
			templateDeleted: []bool{false},
			eligible:        []bool{false},
		},
		{
			name:         "prebuild claimed",
			transitions:  allTransitions,
			jobStatuses:  allJobStatuses,
			initiatorIDs: []uuid.UUID{agplprebuilds.SystemUserID},
			ownerIDs:     []uuid.UUID{uuid.New()},
			metrics: []metricCheck{
				{"coderd_prebuilt_workspaces_created_total", ptr.To(1.0), true},
				{"coderd_prebuilt_workspaces_claimed_total", ptr.To(1.0), true},
				{"coderd_prebuilt_workspaces_desired", ptr.To(1.0), false},
				{"coderd_prebuilt_workspaces_running", ptr.To(0.0), false},
				{"coderd_prebuilt_workspaces_eligible", ptr.To(0.0), false},
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
				{"coderd_prebuilt_workspaces_desired", ptr.To(1.0), false},
				{"coderd_prebuilt_workspaces_running", ptr.To(0.0), false},
				{"coderd_prebuilt_workspaces_eligible", ptr.To(0.0), false},
			},
			templateDeleted: []bool{false},
			eligible:        []bool{false},
		},
		{
			name:         "deleted templates never desire prebuilds",
			transitions:  allTransitions,
			jobStatuses:  allJobStatuses,
			initiatorIDs: []uuid.UUID{agplprebuilds.SystemUserID},
			ownerIDs:     []uuid.UUID{agplprebuilds.SystemUserID, uuid.New()},
			metrics: []metricCheck{
				{"coderd_prebuilt_workspaces_desired", ptr.To(0.0), false},
			},
			templateDeleted: []bool{true},
			eligible:        []bool{false},
		},
		{
			name:         "running prebuilds for deleted templates are still counted, so that they can be deleted",
			transitions:  []database.WorkspaceTransition{database.WorkspaceTransitionStart},
			jobStatuses:  []database.ProvisionerJobStatus{database.ProvisionerJobStatusSucceeded},
			initiatorIDs: []uuid.UUID{agplprebuilds.SystemUserID},
			ownerIDs:     []uuid.UUID{agplprebuilds.SystemUserID},
			metrics: []metricCheck{
				{"coderd_prebuilt_workspaces_running", ptr.To(1.0), false},
				{"coderd_prebuilt_workspaces_eligible", ptr.To(0.0), false},
			},
			templateDeleted: []bool{true},
			eligible:        []bool{false},
		},
	}
	for _, test := range tests {
		test := test // capture for parallel
		for _, transition := range test.transitions {
			transition := transition // capture for parallel
			for _, jobStatus := range test.jobStatuses {
				jobStatus := jobStatus // capture for parallel
				for _, initiatorID := range test.initiatorIDs {
					initiatorID := initiatorID // capture for parallel
					for _, ownerID := range test.ownerIDs {
						ownerID := ownerID // capture for parallel
						for _, templateDeleted := range test.templateDeleted {
							templateDeleted := templateDeleted // capture for parallel
							for _, eligible := range test.eligible {
								eligible := eligible // capture for parallel
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
									reconciler := prebuilds.NewStoreReconciler(db, pubsub, codersdk.PrebuildsConfig{}, logger, quartz.NewMock(t), prometheus.NewRegistry())
									ctx := testutil.Context(t, testutil.WaitLong)

									createdUsers := []uuid.UUID{agplprebuilds.SystemUserID}
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
										workspace := setupTestDBWorkspace(
											t, clock, db, pubsub,
											transition, jobStatus, org.ID, preset, template.ID, templateVersionID, initiatorID, ownerID,
										)
										setupTestDBWorkspaceAgent(t, db, workspace.ID, eligible)
									}

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
											preset := preset // capture for parallel
											labels := map[string]string{
												"template_name":     template.Name,
												"preset_name":       preset.Name,
												"organization_name": org.Name,
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
