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
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type metricCheck struct {
	name      string
	value     *float64
	isCounter bool
}

func TestMetricsCollector(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("this test requires postgres")
	}

	type testCase struct {
		name                             string
		transitions                      []database.WorkspaceTransition
		jobStatuses                      []database.ProvisionerJobStatus
		initiatorIDs                     []uuid.UUID
		ownerIDs                         []uuid.UUID
		shouldIncrementPrebuildsCreated  *float64
		shouldIncrementPrebuildsFailed   *float64
		shouldIncrementPrebuildsAssigned *float64
		shouldSetDesiredPrebuilds        *float64
		shouldSetActualPrebuilds         *float64
		shouldSetEligiblePrebuilds       *float64
	}

	tests := []testCase{
		{
			name: "prebuild created",
			// A prebuild is a workspace, for which the first build was a start transition
			// initiated by the prebuilds user. Whether or not the build was successful, it
			// is still a prebuild. It might just not be a running prebuild.
			transitions:                     allTransitions,
			jobStatuses:                     allJobStatuses,
			initiatorIDs:                    []uuid.UUID{prebuilds.OwnerID},
			ownerIDs:                        []uuid.UUID{prebuilds.OwnerID, uuid.New()},
			shouldIncrementPrebuildsCreated: ptr.To(1.0),
			shouldSetDesiredPrebuilds:       ptr.To(1.0),
			shouldSetEligiblePrebuilds:      ptr.To(0.0),
		},
		{
			name:                            "prebuild running",
			transitions:                     []database.WorkspaceTransition{database.WorkspaceTransitionStart},
			jobStatuses:                     []database.ProvisionerJobStatus{database.ProvisionerJobStatusSucceeded},
			initiatorIDs:                    []uuid.UUID{prebuilds.OwnerID},
			ownerIDs:                        []uuid.UUID{prebuilds.OwnerID},
			shouldIncrementPrebuildsCreated: ptr.To(1.0),
			shouldSetDesiredPrebuilds:       ptr.To(1.0),
			shouldSetActualPrebuilds:        ptr.To(1.0),
			shouldSetEligiblePrebuilds:      ptr.To(0.0),
		},
		{
			name:                            "prebuild failed",
			transitions:                     allTransitions,
			jobStatuses:                     []database.ProvisionerJobStatus{database.ProvisionerJobStatusFailed},
			initiatorIDs:                    []uuid.UUID{prebuilds.OwnerID},
			ownerIDs:                        []uuid.UUID{prebuilds.OwnerID, uuid.New()},
			shouldIncrementPrebuildsCreated: ptr.To(1.0),
			shouldIncrementPrebuildsFailed:  ptr.To(1.0),
			shouldSetDesiredPrebuilds:       ptr.To(1.0),
			shouldSetActualPrebuilds:        ptr.To(0.0),
			shouldSetEligiblePrebuilds:      ptr.To(0.0),
		},
		{
			name:                             "prebuild assigned",
			transitions:                      allTransitions,
			jobStatuses:                      allJobStatuses,
			initiatorIDs:                     []uuid.UUID{prebuilds.OwnerID},
			ownerIDs:                         []uuid.UUID{uuid.New()},
			shouldIncrementPrebuildsCreated:  ptr.To(1.0),
			shouldIncrementPrebuildsAssigned: ptr.To(1.0),
			shouldSetDesiredPrebuilds:        ptr.To(1.0),
			shouldSetActualPrebuilds:         ptr.To(0.0),
			shouldSetEligiblePrebuilds:       ptr.To(0.0),
		},
		{
			name:                             "workspaces that were not created by the prebuilds user are not counted",
			transitions:                      allTransitions,
			jobStatuses:                      allJobStatuses,
			initiatorIDs:                     []uuid.UUID{uuid.New()},
			ownerIDs:                         []uuid.UUID{uuid.New()},
			shouldIncrementPrebuildsCreated:  nil,
			shouldIncrementPrebuildsFailed:   nil,
			shouldIncrementPrebuildsAssigned: nil,
			shouldSetDesiredPrebuilds:        ptr.To(1.0),
			shouldSetActualPrebuilds:         ptr.To(0.0),
			shouldSetEligiblePrebuilds:       ptr.To(0.0),
		},
	}
	for _, test := range tests {
		for _, transition := range test.transitions {
			for _, jobStatus := range test.jobStatuses {
				for _, initiatorID := range test.initiatorIDs {
					for _, ownerID := range test.ownerIDs {
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

							createdUsers := []uuid.UUID{prebuilds.OwnerID}
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
								templateVersionID := setupTestDBTemplateVersion(t, ctx, clock, db, pubsub, orgID, ownerID, templateID)
								preset := setupTestDBPreset(t, ctx, db, pubsub, templateVersionID, 1, uuid.New().String())
								setupTestDBWorkspace(
									t, ctx, clock, db, pubsub,
									transition, jobStatus, orgID, preset, templateID, templateVersionID, initiatorID, ownerID,
								)
							}

							metricsFamilies, err := registry.Gather()
							require.NoError(t, err)

							templates, err := db.GetTemplates(ctx)
							require.NoError(t, err)
							require.Equal(t, numTemplates, len(templates))

							for _, template := range templates {
								templateVersions, err := db.GetTemplateVersionsByTemplateID(ctx, database.GetTemplateVersionsByTemplateIDParams{
									TemplateID: template.ID,
								})
								require.NoError(t, err)
								require.Equal(t, 1, len(templateVersions))

								presets, err := db.GetPresetsByTemplateVersionID(ctx, templateVersions[0].ID)
								require.NoError(t, err)
								require.Equal(t, 1, len(presets))

								for _, preset := range presets {
									checks := []metricCheck{
										{"coderd_prebuilds_created", test.shouldIncrementPrebuildsCreated, true},
										{"coderd_prebuilds_failed", test.shouldIncrementPrebuildsFailed, true},
										{"coderd_prebuilds_claimed", test.shouldIncrementPrebuildsAssigned, true},
										{"coderd_prebuilds_desired", test.shouldSetDesiredPrebuilds, false},
										{"coderd_prebuilds_running", test.shouldSetActualPrebuilds, false},
										{"coderd_prebuilds_eligible", test.shouldSetEligiblePrebuilds, false},
									}

									labels := map[string]string{
										"template_name": template.Name,
										"preset_name":   preset.Name,
									}

									for _, check := range checks {
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
		if metricFamily.GetName() == name {
			for _, metric := range metricFamily.GetMetric() {
				matches := true
				labelPairs := metric.GetLabel()

				// Check if all requested labels match
				for wantName, wantValue := range labels {
					found := false
					for _, label := range labelPairs {
						if label.GetName() == wantName && label.GetValue() == wantValue {
							found = true
							break
						}
					}
					if !found {
						matches = false
						break
					}
				}

				if matches {
					return metric
				}
			}
		}
	}
	return nil
}
