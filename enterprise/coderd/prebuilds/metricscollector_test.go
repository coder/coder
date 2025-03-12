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
		name         string
		transitions  []database.WorkspaceTransition
		jobStatuses  []database.ProvisionerJobStatus
		initiatorIDs []uuid.UUID
		ownerIDs     []uuid.UUID
		metrics      []metricCheck
	}

	tests := []testCase{
		{
			name:         "prebuild created",
			transitions:  allTransitions,
			jobStatuses:  allJobStatuses,
			initiatorIDs: []uuid.UUID{agplprebuilds.OwnerID},
			// TODO: reexamine and refactor the test cases and assertions:
			// * a running prebuild that is not elibible to be claimed currently seems to be eligible.
			// * a prebuild that was claimed should not be deemed running, not eligible.
			ownerIDs: []uuid.UUID{agplprebuilds.OwnerID, uuid.New()},
			metrics: []metricCheck{
				{"coderd_prebuilds_created", ptr.To(1.0), true},
				{"coderd_prebuilds_desired", ptr.To(1.0), false},
				// {"coderd_prebuilds_running", ptr.To(0.0), false},
				// {"coderd_prebuilds_eligible", ptr.To(0.0), false},
			},
		},
		{
			name:         "prebuild running",
			transitions:  []database.WorkspaceTransition{database.WorkspaceTransitionStart},
			jobStatuses:  []database.ProvisionerJobStatus{database.ProvisionerJobStatusSucceeded},
			initiatorIDs: []uuid.UUID{agplprebuilds.OwnerID},
			ownerIDs:     []uuid.UUID{agplprebuilds.OwnerID},
			metrics: []metricCheck{
				{"coderd_prebuilds_created", ptr.To(1.0), true},
				{"coderd_prebuilds_desired", ptr.To(1.0), false},
				{"coderd_prebuilds_running", ptr.To(1.0), false},
				{"coderd_prebuilds_eligible", ptr.To(0.0), false},
			},
		},
		{
			name:         "prebuild failed",
			transitions:  allTransitions,
			jobStatuses:  []database.ProvisionerJobStatus{database.ProvisionerJobStatusFailed},
			initiatorIDs: []uuid.UUID{agplprebuilds.OwnerID},
			ownerIDs:     []uuid.UUID{agplprebuilds.OwnerID, uuid.New()},
			metrics: []metricCheck{
				{"coderd_prebuilds_created", ptr.To(1.0), true},
				{"coderd_prebuilds_failed", ptr.To(1.0), true},
				{"coderd_prebuilds_desired", ptr.To(1.0), false},
				{"coderd_prebuilds_running", ptr.To(0.0), false},
				{"coderd_prebuilds_eligible", ptr.To(0.0), false},
			},
		},
		{
			name:         "prebuild assigned",
			transitions:  allTransitions,
			jobStatuses:  allJobStatuses,
			initiatorIDs: []uuid.UUID{agplprebuilds.OwnerID},
			ownerIDs:     []uuid.UUID{uuid.New()},
			metrics: []metricCheck{
				{"coderd_prebuilds_created", ptr.To(1.0), true},
				{"coderd_prebuilds_claimed", ptr.To(1.0), true},
				{"coderd_prebuilds_desired", ptr.To(1.0), false},
				{"coderd_prebuilds_running", ptr.To(0.0), false},
				{"coderd_prebuilds_eligible", ptr.To(0.0), false},
			},
		},
		{
			name:         "workspaces that were not created by the prebuilds user are not counted",
			transitions:  allTransitions,
			jobStatuses:  allJobStatuses,
			initiatorIDs: []uuid.UUID{uuid.New()},
			ownerIDs:     []uuid.UUID{uuid.New()},
			metrics: []metricCheck{
				{"coderd_prebuilds_desired", ptr.To(1.0), false},
				{"coderd_prebuilds_running", ptr.To(0.0), false},
				{"coderd_prebuilds_eligible", ptr.To(0.0), false},
			},
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
						t.Run(fmt.Sprintf("transition:%s/jobStatus:%s", transition, jobStatus), func(t *testing.T) {
							t.Parallel()

							logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
							t.Cleanup(func() {
								if t.Failed() {
									t.Logf("failed to run test: %s", test.name)
									t.Logf("transition: %s", transition)
									t.Logf("jobStatus: %s", jobStatus)
									t.Logf("initiatorID: %s", initiatorID)
									t.Logf("ownerID: %s", ownerID)
								}
							})
							clock := quartz.NewMock(t)
							db, pubsub := dbtestutil.NewDB(t)
							reconciler := prebuilds.NewStoreReconciler(db, pubsub, codersdk.PrebuildsConfig{}, logger, quartz.NewMock(t))
							ctx := testutil.Context(t, testutil.WaitLong)

							createdUsers := []uuid.UUID{agplprebuilds.OwnerID}
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
								orgID, templateID := setupTestDBTemplate(t, db, ownerID)
								templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, pubsub, orgID, ownerID, templateID)
								preset := setupTestDBPreset(t, db, templateVersionID, 1, uuid.New().String())
								setupTestDBWorkspace(
									t, clock, db, pubsub,
									transition, jobStatus, orgID, preset, templateID, templateVersionID, initiatorID, ownerID,
								)
							}

							metricsFamilies, err := registry.Gather()
							require.NoError(t, err)

							templates, err := db.GetTemplates(ctx)
							require.NoError(t, err)
							require.Equal(t, numTemplates, len(templates))

							for _, template := range templates {
								template := template // capture for parallel
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
										"template_name": template.Name,
										"preset_name":   preset.Name,
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
