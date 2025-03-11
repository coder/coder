package prebuilds_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tailscale.com/types/ptr"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/slogtest"
	"github.com/coder/quartz"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/testutil"
)

func TestNoReconciliationActionsIfNoPresets(t *testing.T) {
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	// Scenario: No reconciliation actions are taken if there are no presets
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, pubsub := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval: serpent.Duration(testutil.WaitLong),
	}
	logger := testutil.Logger(t)
	controller := prebuilds.NewStoreReconciler(db, pubsub, cfg, logger, quartz.NewMock(t))

	// given a template version with no presets
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	// verify that the db state is correct
	gotTemplateVersion, err := db.GetTemplateVersionByID(ctx, templateVersion.ID)
	require.NoError(t, err)
	require.Equal(t, templateVersion, gotTemplateVersion)

	// when we trigger the reconciliation loop for all templates
	require.NoError(t, controller.ReconcileAll(ctx))

	// then no reconciliation actions are taken
	// because without presets, there are no prebuilds
	// and without prebuilds, there is nothing to reconcile
	jobs, err := db.GetProvisionerJobsCreatedAfter(ctx, time.Now().Add(earlier))
	require.NoError(t, err)
	require.Empty(t, jobs)
}

func TestNoReconciliationActionsIfNoPrebuilds(t *testing.T) {
	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	// Scenario: No reconciliation actions are taken if there are no prebuilds
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, pubsub := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval: serpent.Duration(testutil.WaitLong),
	}
	logger := testutil.Logger(t)
	controller := prebuilds.NewStoreReconciler(db, pubsub, cfg, logger, quartz.NewMock(t))

	// given there are presets, but no prebuilds
	org := dbgen.Organization(t, db, database.Organization{})
	user := dbgen.User(t, db, database.User{})
	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      user.ID,
		OrganizationID: org.ID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: template.ID, Valid: true},
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	preset, err := db.InsertPreset(ctx, database.InsertPresetParams{
		TemplateVersionID: templateVersion.ID,
		Name:              "test",
	})
	require.NoError(t, err)
	_, err = db.InsertPresetParameters(ctx, database.InsertPresetParametersParams{
		TemplateVersionPresetID: preset.ID,
		Names:                   []string{"test"},
		Values:                  []string{"test"},
	})
	require.NoError(t, err)

	// verify that the db state is correct
	presetParameters, err := db.GetPresetParametersByTemplateVersionID(ctx, templateVersion.ID)
	require.NoError(t, err)
	require.NotEmpty(t, presetParameters)

	// when we trigger the reconciliation loop for all templates
	require.NoError(t, controller.ReconcileAll(ctx))

	// then no reconciliation actions are taken
	// because without prebuilds, there is nothing to reconcile
	// even if there are presets
	jobs, err := db.GetProvisionerJobsCreatedAfter(ctx, time.Now().Add(earlier))
	require.NoError(t, err)
	require.Empty(t, jobs)
}

func TestPrebuildReconciliation(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	type testCase struct {
		name                      string
		prebuildLatestTransitions []database.WorkspaceTransition
		prebuildJobStatuses       []database.ProvisionerJobStatus
		templateVersionActive     []bool
		shouldCreateNewPrebuild   *bool
		shouldDeleteOldPrebuild   *bool
	}

	testCases := []testCase{
		{
			name:                      "never create prebuilds for inactive template versions",
			prebuildLatestTransitions: allTransitions,
			prebuildJobStatuses:       allJobStatuses,
			templateVersionActive:     []bool{false},
			shouldCreateNewPrebuild:   ptr.To(false),
		},
		{
			name: "no need to create a new prebuild if one is already running",
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStart,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusSucceeded,
			},
			templateVersionActive:   []bool{true},
			shouldCreateNewPrebuild: ptr.To(false),
		},
		{
			name: "don't create a new prebuild if one is queued to build or already building",
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStart,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusPending,
				database.ProvisionerJobStatusRunning,
			},
			templateVersionActive:   []bool{true},
			shouldCreateNewPrebuild: ptr.To(false),
		},
		{
			name: "create a new prebuild if one is in a state that disqualifies it from ever being claimed",
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStop,
				database.WorkspaceTransitionDelete,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusPending,
				database.ProvisionerJobStatusRunning,
				database.ProvisionerJobStatusCanceling,
				database.ProvisionerJobStatusSucceeded,
			},
			templateVersionActive:   []bool{true},
			shouldCreateNewPrebuild: ptr.To(true),
		},
		{
			name:                      "create a new prebuild if one is in any kind of exceptional state",
			prebuildLatestTransitions: allTransitions,
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusCanceled,
				database.ProvisionerJobStatusFailed,
			},
			templateVersionActive:   []bool{true},
			shouldCreateNewPrebuild: ptr.To(true),
		},
		{
			name: "never attempt to interfere with active builds",
			// The workspace builder does not allow scheduling a new build if there is already a build
			// pending, running, or canceling. As such, we should never attempt to start, stop or delete
			// such prebuilds. Rather, we should wait for the existing build to complete and reconcile
			// again in the next cycle.
			prebuildLatestTransitions: allTransitions,
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusPending,
				database.ProvisionerJobStatusRunning,
				database.ProvisionerJobStatusCanceling,
			},
			templateVersionActive:   []bool{true, false},
			shouldDeleteOldPrebuild: ptr.To(false),
		},
		{
			name: "never delete prebuilds in an exceptional state",
			// We don't want to destroy evidence that might be useful to operators
			// when troubleshooting issues. So we leave these prebuilds in place.
			// Operators are expected to manually delete these prebuilds.
			prebuildLatestTransitions: allTransitions,
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusCanceled,
				database.ProvisionerJobStatusFailed,
			},
			templateVersionActive:   []bool{true, false},
			shouldDeleteOldPrebuild: ptr.To(false),
		},
		{
			name: "delete running prebuilds for inactive template versions",
			// We only support prebuilds for active template versions.
			// If a template version is inactive, we should delete any prebuilds
			// that are running.
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStart,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusSucceeded,
			},
			templateVersionActive:   []bool{false},
			shouldDeleteOldPrebuild: ptr.To(true),
		},
		{
			name: "don't delete running prebuilds for active template versions",
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStart,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusSucceeded,
			},
			templateVersionActive:   []bool{true},
			shouldDeleteOldPrebuild: ptr.To(false),
		},
		{
			name: "don't delete stopped or already deleted prebuilds",
			// We don't ever stop prebuilds. A stopped prebuild is an exceptional state.
			// As such we keep it, to allow operators to investigate the cause.
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStop,
				database.WorkspaceTransitionDelete,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusSucceeded,
			},
			templateVersionActive:   []bool{true, false},
			shouldDeleteOldPrebuild: ptr.To(false),
		},
	}
	for _, tc := range testCases {
		for _, templateVersionActive := range tc.templateVersionActive {
			for _, prebuildLatestTransition := range tc.prebuildLatestTransitions {
				for _, prebuildJobStatus := range tc.prebuildJobStatuses {
					t.Run(fmt.Sprintf("%s - %s - %s", tc.name, prebuildLatestTransition, prebuildJobStatus), func(t *testing.T) {
						t.Parallel()
						t.Cleanup(func() {
							if t.Failed() {
								t.Logf("failed to run test: %s", tc.name)
								t.Logf("templateVersionActive: %t", templateVersionActive)
								t.Logf("prebuildLatestTransition: %s", prebuildLatestTransition)
								t.Logf("prebuildJobStatus: %s", prebuildJobStatus)
							}
						})
						ctx := testutil.Context(t, testutil.WaitShort)
						cfg := codersdk.PrebuildsConfig{}
						logger := slogtest.Make(
							t, &slogtest.Options{IgnoreErrors: true},
						).Leveled(slog.LevelDebug)
						db, pubsub := dbtestutil.NewDB(t)
						controller := prebuilds.NewStoreReconciler(db, pubsub, cfg, logger, quartz.NewMock(t))

						ownerID := uuid.New()
						dbgen.User(t, db, database.User{
							ID: ownerID,
						})
						orgID, templateID := setupTestDBTemplate(t, db, ownerID)
						templateVersionID := setupTestDBTemplateVersion(
							t,
							ctx,
							db,
							pubsub,
							orgID,
							ownerID,
							templateID,
						)
						preset := setupTestDBPreset(
							t,
							ctx,
							db,
							pubsub,
							templateVersionID,
							1,
						)
						prebuildID := setupTestDBPrebuild(
							t,
							ctx,
							db,
							pubsub,
							prebuildLatestTransition,
							prebuildJobStatus,
							orgID,
							templateID,
							templateVersionID,
							preset.ID,
							prebuilds.OwnerID,
							prebuilds.OwnerID,
						)

						if !templateVersionActive {
							// Create a new template version and mark it as active
							// This marks the template version that we care about as inactive
							setupTestDBTemplateVersion(
								t,
								ctx,
								db,
								pubsub,
								orgID,
								ownerID,
								templateID,
							)
						}

						// Run the reconciliation multiple times to ensure idempotency
						// 8 was arbitrary, but large enough to reasonably trust the result
						for i := 1; i <= 8; i++ {
							require.NoErrorf(t, controller.ReconcileAll(ctx), "failed on iteration %d", i)

							if tc.shouldCreateNewPrebuild != nil {
								newPrebuildCount := 0
								workspaces, err := db.GetWorkspacesByTemplateID(ctx, templateID)
								require.NoError(t, err)
								for _, workspace := range workspaces {
									if workspace.ID != prebuildID {
										newPrebuildCount++
									}
								}
								// This test configures a preset that desires one prebuild.
								// In cases where new prebuilds should be created, there should be exactly one.
								require.Equal(t, *tc.shouldCreateNewPrebuild, newPrebuildCount == 1)
							}

							if tc.shouldDeleteOldPrebuild != nil {
								builds, err := db.GetWorkspaceBuildsByWorkspaceID(ctx, database.GetWorkspaceBuildsByWorkspaceIDParams{
									WorkspaceID: prebuildID,
								})
								require.NoError(t, err)
								if *tc.shouldDeleteOldPrebuild {
									require.Equal(t, 2, len(builds))
									require.Equal(t, database.WorkspaceTransitionDelete, builds[0].Transition)
								} else {
									require.Equal(t, 1, len(builds))
									require.Equal(t, prebuildLatestTransition, builds[0].Transition)
								}
							}
						}
					})
				}
			}
		}
	}
}

func setupTestDBTemplate(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
) (
	orgID uuid.UUID,
	templateID uuid.UUID,
) {
	t.Helper()
	org := dbgen.Organization(t, db, database.Organization{})

	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      userID,
		OrganizationID: org.ID,
	})

	return org.ID, template.ID
}

const (
	earlier     = -time.Hour
	muchEarlier = time.Hour * -2
)

func setupTestDBTemplateVersion(
	t *testing.T,
	ctx context.Context,
	db database.Store,
	ps pubsub.Pubsub,
	orgID uuid.UUID,
	userID uuid.UUID,
	templateID uuid.UUID,
) uuid.UUID {
	t.Helper()
	templateVersionJob := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		CreatedAt:      time.Now().Add(muchEarlier),
		CompletedAt:    sql.NullTime{Time: time.Now().Add(earlier), Valid: true},
		OrganizationID: orgID,
		InitiatorID:    userID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: templateID, Valid: true},
		OrganizationID: orgID,
		CreatedBy:      userID,
		JobID:          templateVersionJob.ID,
	})
	require.NoError(t, db.UpdateTemplateActiveVersionByID(ctx, database.UpdateTemplateActiveVersionByIDParams{
		ID:              templateID,
		ActiveVersionID: templateVersion.ID,
	}))
	return templateVersion.ID
}

func setupTestDBPreset(
	t *testing.T,
	ctx context.Context,
	db database.Store,
	ps pubsub.Pubsub,
	templateVersionID uuid.UUID,
	desiredInstances int32,
) database.TemplateVersionPreset {
	t.Helper()
	preset := dbgen.Preset(t, db, database.InsertPresetParams{
		TemplateVersionID: templateVersionID,
		Name:              "test",
	})
	dbgen.PresetParameter(t, db, database.InsertPresetParametersParams{
		TemplateVersionPresetID: preset.ID,
		Names:                   []string{"test"},
		Values:                  []string{"test"},
	})
	_, err := db.InsertPresetPrebuild(ctx, database.InsertPresetPrebuildParams{
		ID:               uuid.New(),
		PresetID:         preset.ID,
		DesiredInstances: desiredInstances,
	})
	require.NoError(t, err)
	return preset
}

func setupTestDBPrebuild(
	t *testing.T,
	ctx context.Context,
	db database.Store,
	ps pubsub.Pubsub,
	transition database.WorkspaceTransition,
	prebuildStatus database.ProvisionerJobStatus,
	orgID uuid.UUID,
	templateID uuid.UUID,
	templateVersionID uuid.UUID,
	presetID uuid.UUID,
	initiatorID uuid.UUID,
	ownerID uuid.UUID,
) (
	prebuildID uuid.UUID,
) {
	cancelledAt := sql.NullTime{}
	completedAt := sql.NullTime{}

	startedAt := sql.NullTime{}
	if prebuildStatus != database.ProvisionerJobStatusPending {
		startedAt = sql.NullTime{Time: time.Now().Add(muchEarlier), Valid: true}
	}

	buildError := sql.NullString{}
	if prebuildStatus == database.ProvisionerJobStatusFailed {
		completedAt = sql.NullTime{Time: time.Now().Add(earlier), Valid: true}
		buildError = sql.NullString{String: "build failed", Valid: true}
	}

	switch prebuildStatus {
	case database.ProvisionerJobStatusCanceling:
		cancelledAt = sql.NullTime{Time: time.Now().Add(earlier), Valid: true}
	case database.ProvisionerJobStatusCanceled:
		completedAt = sql.NullTime{Time: time.Now().Add(earlier), Valid: true}
		cancelledAt = sql.NullTime{Time: time.Now().Add(earlier), Valid: true}
	case database.ProvisionerJobStatusSucceeded:
		completedAt = sql.NullTime{Time: time.Now().Add(earlier), Valid: true}
	default:
	}

	workspace := dbgen.Workspace(t, db, database.WorkspaceTable{
		TemplateID:     templateID,
		OrganizationID: orgID,
		OwnerID:        ownerID,
		Deleted:        false,
	})
	job := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		InitiatorID:    initiatorID,
		CreatedAt:      time.Now().Add(muchEarlier),
		StartedAt:      startedAt,
		CompletedAt:    completedAt,
		CanceledAt:     cancelledAt,
		OrganizationID: orgID,
		Error:          buildError,
	})
	dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:             workspace.ID,
		InitiatorID:             initiatorID,
		TemplateVersionID:       templateVersionID,
		JobID:                   job.ID,
		TemplateVersionPresetID: uuid.NullUUID{UUID: presetID, Valid: true},
		Transition:              transition,
	})

	return workspace.ID
}

var allTransitions = []database.WorkspaceTransition{
	database.WorkspaceTransitionStart,
	database.WorkspaceTransitionStop,
	database.WorkspaceTransitionDelete,
}

var allJobStatuses = []database.ProvisionerJobStatus{
	database.ProvisionerJobStatusPending,
	database.ProvisionerJobStatusRunning,
	database.ProvisionerJobStatusSucceeded,
	database.ProvisionerJobStatusFailed,
	database.ProvisionerJobStatusCanceled,
	database.ProvisionerJobStatusCanceling,
}

// TODO (sasswart): test mutual exclusion
