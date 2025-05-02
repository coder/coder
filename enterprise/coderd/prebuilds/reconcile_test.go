package prebuilds_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/util/slice"

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
	agplprebuilds "github.com/coder/coder/v2/coderd/prebuilds"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/prebuilds"
	"github.com/coder/coder/v2/testutil"
)

func TestNoReconciliationActionsIfNoPresets(t *testing.T) {
	// Scenario: No reconciliation actions are taken if there are no presets
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval: serpent.Duration(testutil.WaitLong),
	}
	logger := testutil.Logger(t)
	controller := prebuilds.NewStoreReconciler(db, ps, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry())

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
	jobs, err := db.GetProvisionerJobsCreatedAfter(ctx, clock.Now().Add(earlier))
	require.NoError(t, err)
	require.Empty(t, jobs)
}

func TestNoReconciliationActionsIfNoPrebuilds(t *testing.T) {
	// Scenario: No reconciliation actions are taken if there are no prebuilds
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitLong)
	db, ps := dbtestutil.NewDB(t)
	cfg := codersdk.PrebuildsConfig{
		ReconciliationInterval: serpent.Duration(testutil.WaitLong),
	}
	logger := testutil.Logger(t)
	controller := prebuilds.NewStoreReconciler(db, ps, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry())

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
	jobs, err := db.GetProvisionerJobsCreatedAfter(ctx, clock.Now().Add(earlier))
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
		templateDeleted           []bool
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
			templateDeleted:           []bool{false},
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
			templateDeleted:         []bool{false},
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
			templateDeleted:         []bool{false},
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
			templateDeleted:         []bool{false},
		},
		{
			// See TestFailedBuildBackoff for the start/failed case.
			name: "create a new prebuild if one is in any kind of exceptional state",
			prebuildLatestTransitions: []database.WorkspaceTransition{
				database.WorkspaceTransitionStop,
				database.WorkspaceTransitionDelete,
			},
			prebuildJobStatuses: []database.ProvisionerJobStatus{
				database.ProvisionerJobStatusCanceled,
			},
			templateVersionActive:   []bool{true},
			shouldCreateNewPrebuild: ptr.To(true),
			templateDeleted:         []bool{false},
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
			templateDeleted:         []bool{false},
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
			templateDeleted:         []bool{false},
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
			templateDeleted:         []bool{false},
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
			templateDeleted:         []bool{false},
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
			templateDeleted:         []bool{false},
		},
		{
			name:                      "delete prebuilds for deleted templates",
			prebuildLatestTransitions: []database.WorkspaceTransition{database.WorkspaceTransitionStart},
			prebuildJobStatuses:       []database.ProvisionerJobStatus{database.ProvisionerJobStatusSucceeded},
			templateVersionActive:     []bool{true, false},
			shouldDeleteOldPrebuild:   ptr.To(true),
			templateDeleted:           []bool{true},
		},
	}
	for _, tc := range testCases {
		tc := tc // capture for parallel
		for _, templateVersionActive := range tc.templateVersionActive {
			for _, prebuildLatestTransition := range tc.prebuildLatestTransitions {
				for _, prebuildJobStatus := range tc.prebuildJobStatuses {
					for _, templateDeleted := range tc.templateDeleted {
						for _, useBrokenPubsub := range []bool{true, false} {
							t.Run(fmt.Sprintf("%s - %s - %s - pubsub_broken=%v", tc.name, prebuildLatestTransition, prebuildJobStatus, useBrokenPubsub), func(t *testing.T) {
								t.Parallel()
								t.Cleanup(func() {
									if t.Failed() {
										t.Logf("failed to run test: %s", tc.name)
										t.Logf("templateVersionActive: %t", templateVersionActive)
										t.Logf("prebuildLatestTransition: %s", prebuildLatestTransition)
										t.Logf("prebuildJobStatus: %s", prebuildJobStatus)
									}
								})
								clock := quartz.NewMock(t)
								ctx := testutil.Context(t, testutil.WaitShort)
								cfg := codersdk.PrebuildsConfig{}
								logger := slogtest.Make(
									t, &slogtest.Options{IgnoreErrors: true},
								).Leveled(slog.LevelDebug)
								db, pubSub := dbtestutil.NewDB(t)
								if useBrokenPubsub {
									pubSub = &brokenPublisher{Pubsub: pubSub}
								}

								controller := prebuilds.NewStoreReconciler(db, pubSub, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry())

								ownerID := uuid.New()
								dbgen.User(t, db, database.User{
									ID: ownerID,
								})
								org, template := setupTestDBTemplate(t, db, ownerID, templateDeleted)
								templateVersionID := setupTestDBTemplateVersion(
									ctx,
									t,
									clock,
									db,
									pubSub,
									org.ID,
									ownerID,
									template.ID,
								)
								preset := setupTestDBPreset(
									t,
									db,
									templateVersionID,
									1,
									uuid.New().String(),
								)
								prebuild := setupTestDBPrebuild(
									t,
									clock,
									db,
									pubSub,
									prebuildLatestTransition,
									prebuildJobStatus,
									org.ID,
									preset,
									template.ID,
									templateVersionID,
								)

								if !templateVersionActive {
									// Create a new template version and mark it as active
									// This marks the template version that we care about as inactive
									setupTestDBTemplateVersion(ctx, t, clock, db, pubSub, org.ID, ownerID, template.ID)
								}

								// Run the reconciliation multiple times to ensure idempotency
								// 8 was arbitrary, but large enough to reasonably trust the result
								for i := 1; i <= 8; i++ {
									require.NoErrorf(t, controller.ReconcileAll(ctx), "failed on iteration %d", i)

									if tc.shouldCreateNewPrebuild != nil {
										newPrebuildCount := 0
										workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
										require.NoError(t, err)
										for _, workspace := range workspaces {
											if workspace.ID != prebuild.ID {
												newPrebuildCount++
											}
										}
										// This test configures a preset that desires one prebuild.
										// In cases where new prebuilds should be created, there should be exactly one.
										require.Equal(t, *tc.shouldCreateNewPrebuild, newPrebuildCount == 1)
									}

									if tc.shouldDeleteOldPrebuild != nil {
										builds, err := db.GetWorkspaceBuildsByWorkspaceID(ctx, database.GetWorkspaceBuildsByWorkspaceIDParams{
											WorkspaceID: prebuild.ID,
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
	}
}

// brokenPublisher is used to validate that Publish() calls which always fail do not affect the reconciler's behavior,
// since the messages published are not essential but merely advisory.
type brokenPublisher struct {
	pubsub.Pubsub
}

func (*brokenPublisher) Publish(event string, _ []byte) error {
	// I'm explicitly _not_ checking for EventJobPosted (coderd/database/provisionerjobs/provisionerjobs.go) since that
	// requires too much knowledge of the underlying implementation.
	return xerrors.Errorf("refusing to publish %q", event)
}

func TestMultiplePresetsPerTemplateVersion(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	prebuildLatestTransition := database.WorkspaceTransitionStart
	prebuildJobStatus := database.ProvisionerJobStatusRunning
	templateDeleted := false

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	cfg := codersdk.PrebuildsConfig{}
	logger := slogtest.Make(
		t, &slogtest.Options{IgnoreErrors: true},
	).Leveled(slog.LevelDebug)
	db, pubSub := dbtestutil.NewDB(t)
	controller := prebuilds.NewStoreReconciler(db, pubSub, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry())

	ownerID := uuid.New()
	dbgen.User(t, db, database.User{
		ID: ownerID,
	})
	org, template := setupTestDBTemplate(t, db, ownerID, templateDeleted)
	templateVersionID := setupTestDBTemplateVersion(
		ctx,
		t,
		clock,
		db,
		pubSub,
		org.ID,
		ownerID,
		template.ID,
	)
	preset := setupTestDBPreset(
		t,
		db,
		templateVersionID,
		4,
		uuid.New().String(),
	)
	preset2 := setupTestDBPreset(
		t,
		db,
		templateVersionID,
		10,
		uuid.New().String(),
	)
	prebuildIDs := make([]uuid.UUID, 0)
	for i := 0; i < int(preset.DesiredInstances.Int32); i++ {
		prebuild := setupTestDBPrebuild(
			t,
			clock,
			db,
			pubSub,
			prebuildLatestTransition,
			prebuildJobStatus,
			org.ID,
			preset,
			template.ID,
			templateVersionID,
		)
		prebuildIDs = append(prebuildIDs, prebuild.ID)
	}

	// Run the reconciliation multiple times to ensure idempotency
	// 8 was arbitrary, but large enough to reasonably trust the result
	for i := 1; i <= 8; i++ {
		require.NoErrorf(t, controller.ReconcileAll(ctx), "failed on iteration %d", i)

		newPrebuildCount := 0
		workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
		require.NoError(t, err)
		for _, workspace := range workspaces {
			if slice.Contains(prebuildIDs, workspace.ID) {
				continue
			}
			newPrebuildCount++
		}

		// NOTE: preset1 doesn't block creation of instances in preset2
		require.Equal(t, preset2.DesiredInstances.Int32, int32(newPrebuildCount)) // nolint:gosec
	}
}

func TestInvalidPreset(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	templateDeleted := false

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	cfg := codersdk.PrebuildsConfig{}
	logger := slogtest.Make(
		t, &slogtest.Options{IgnoreErrors: true},
	).Leveled(slog.LevelDebug)
	db, pubSub := dbtestutil.NewDB(t)
	controller := prebuilds.NewStoreReconciler(db, pubSub, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry())

	ownerID := uuid.New()
	dbgen.User(t, db, database.User{
		ID: ownerID,
	})
	org, template := setupTestDBTemplate(t, db, ownerID, templateDeleted)
	templateVersionID := setupTestDBTemplateVersion(
		ctx,
		t,
		clock,
		db,
		pubSub,
		org.ID,
		ownerID,
		template.ID,
	)
	// Add required param, which is not set in preset. It means that creating of prebuild will constantly fail.
	dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{
		TemplateVersionID: templateVersionID,
		Name:              "required-param",
		Description:       "required param to make sure creating prebuild will fail",
		Type:              "bool",
		DefaultValue:      "",
		Required:          true,
	})
	setupTestDBPreset(
		t,
		db,
		templateVersionID,
		1,
		uuid.New().String(),
	)

	// Run the reconciliation multiple times to ensure idempotency
	// 8 was arbitrary, but large enough to reasonably trust the result
	for i := 1; i <= 8; i++ {
		require.NoErrorf(t, controller.ReconcileAll(ctx), "failed on iteration %d", i)

		workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
		require.NoError(t, err)
		newPrebuildCount := len(workspaces)

		// NOTE: we don't have any new prebuilds, because their creation constantly fails.
		require.Equal(t, int32(0), int32(newPrebuildCount)) // nolint:gosec
	}
}

func TestDeletionOfPrebuiltWorkspaceWithInvalidPreset(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	templateDeleted := false

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	cfg := codersdk.PrebuildsConfig{}
	logger := slogtest.Make(
		t, &slogtest.Options{IgnoreErrors: true},
	).Leveled(slog.LevelDebug)
	db, pubSub := dbtestutil.NewDB(t)
	controller := prebuilds.NewStoreReconciler(db, pubSub, cfg, logger, quartz.NewMock(t), prometheus.NewRegistry())

	ownerID := uuid.New()
	dbgen.User(t, db, database.User{
		ID: ownerID,
	})
	org, template := setupTestDBTemplate(t, db, ownerID, templateDeleted)
	templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, pubSub, org.ID, ownerID, template.ID)
	preset := setupTestDBPreset(t, db, templateVersionID, 1, uuid.New().String())
	prebuiltWorkspace := setupTestDBPrebuild(
		t,
		clock,
		db,
		pubSub,
		database.WorkspaceTransitionStart,
		database.ProvisionerJobStatusSucceeded,
		org.ID,
		preset,
		template.ID,
		templateVersionID,
	)

	workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
	require.NoError(t, err)
	// make sure we have only one workspace
	require.Equal(t, 1, len(workspaces))

	// Create a new template version and mark it as active.
	// This marks the previous template version as inactive.
	templateVersionID = setupTestDBTemplateVersion(ctx, t, clock, db, pubSub, org.ID, ownerID, template.ID)
	// Add required param, which is not set in preset.
	// It means that creating of new prebuilt workspace will fail, but we should be able to clean up old prebuilt workspaces.
	dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{
		TemplateVersionID: templateVersionID,
		Name:              "required-param",
		Description:       "required param which isn't set in preset",
		Type:              "bool",
		DefaultValue:      "",
		Required:          true,
	})

	// Old prebuilt workspace should be deleted.
	require.NoError(t, controller.ReconcileAll(ctx))

	builds, err := db.GetWorkspaceBuildsByWorkspaceID(ctx, database.GetWorkspaceBuildsByWorkspaceIDParams{
		WorkspaceID: prebuiltWorkspace.ID,
	})
	require.NoError(t, err)
	// Make sure old prebuild workspace was deleted, despite it contains required parameter which isn't set in preset.
	require.Equal(t, 2, len(builds))
	require.Equal(t, database.WorkspaceTransitionDelete, builds[0].Transition)
}

func TestRunLoop(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	prebuildLatestTransition := database.WorkspaceTransitionStart
	prebuildJobStatus := database.ProvisionerJobStatusRunning
	templateDeleted := false

	clock := quartz.NewMock(t)
	ctx := testutil.Context(t, testutil.WaitShort)
	backoffInterval := time.Minute
	cfg := codersdk.PrebuildsConfig{
		// Given: explicitly defined backoff configuration to validate timings.
		ReconciliationBackoffLookback: serpent.Duration(muchEarlier * -10), // Has to be positive.
		ReconciliationBackoffInterval: serpent.Duration(backoffInterval),
		ReconciliationInterval:        serpent.Duration(time.Second),
	}
	logger := slogtest.Make(
		t, &slogtest.Options{IgnoreErrors: true},
	).Leveled(slog.LevelDebug)
	db, pubSub := dbtestutil.NewDB(t)
	reconciler := prebuilds.NewStoreReconciler(db, pubSub, cfg, logger, clock, prometheus.NewRegistry())

	ownerID := uuid.New()
	dbgen.User(t, db, database.User{
		ID: ownerID,
	})
	org, template := setupTestDBTemplate(t, db, ownerID, templateDeleted)
	templateVersionID := setupTestDBTemplateVersion(
		ctx,
		t,
		clock,
		db,
		pubSub,
		org.ID,
		ownerID,
		template.ID,
	)
	preset := setupTestDBPreset(
		t,
		db,
		templateVersionID,
		4,
		uuid.New().String(),
	)
	preset2 := setupTestDBPreset(
		t,
		db,
		templateVersionID,
		10,
		uuid.New().String(),
	)
	prebuildIDs := make([]uuid.UUID, 0)
	for i := 0; i < int(preset.DesiredInstances.Int32); i++ {
		prebuild := setupTestDBPrebuild(
			t,
			clock,
			db,
			pubSub,
			prebuildLatestTransition,
			prebuildJobStatus,
			org.ID,
			preset,
			template.ID,
			templateVersionID,
		)
		prebuildIDs = append(prebuildIDs, prebuild.ID)
	}
	getNewPrebuildCount := func() int32 {
		newPrebuildCount := 0
		workspaces, err := db.GetWorkspacesByTemplateID(ctx, template.ID)
		require.NoError(t, err)
		for _, workspace := range workspaces {
			if slice.Contains(prebuildIDs, workspace.ID) {
				continue
			}
			newPrebuildCount++
		}

		return int32(newPrebuildCount) // nolint:gosec
	}

	// we need to wait until ticker is initialized, and only then use clock.Advance()
	// otherwise clock.Advance() will be ignored
	trap := clock.Trap().NewTicker()
	go reconciler.Run(ctx)
	// wait until ticker is initialized
	trap.MustWait(ctx).Release()
	// start 1st iteration of ReconciliationLoop
	// NOTE: at this point MustWait waits that iteration is started (ReconcileAll is called), but it doesn't wait until it completes
	clock.Advance(cfg.ReconciliationInterval.Value()).MustWait(ctx)

	// wait until ReconcileAll is completed
	// TODO: is it possible to avoid Eventually and replace it with quartz?
	// Ideally to have all control on test-level, and be able to advance loop iterations from the test.
	require.Eventually(t, func() bool {
		newPrebuildCount := getNewPrebuildCount()

		// NOTE: preset1 doesn't block creation of instances in preset2
		return preset2.DesiredInstances.Int32 == newPrebuildCount
	}, testutil.WaitShort, testutil.IntervalFast)

	// setup one more preset with 5 prebuilds
	preset3 := setupTestDBPreset(
		t,
		db,
		templateVersionID,
		5,
		uuid.New().String(),
	)
	newPrebuildCount := getNewPrebuildCount()
	// nothing changed, because we didn't trigger a new iteration of a loop
	require.Equal(t, preset2.DesiredInstances.Int32, newPrebuildCount)

	// start 2nd iteration of ReconciliationLoop
	// NOTE: at this point MustWait waits that iteration is started (ReconcileAll is called), but it doesn't wait until it completes
	clock.Advance(cfg.ReconciliationInterval.Value()).MustWait(ctx)

	// wait until ReconcileAll is completed
	require.Eventually(t, func() bool {
		newPrebuildCount := getNewPrebuildCount()

		// both prebuilds for preset2 and preset3 were created
		return preset2.DesiredInstances.Int32+preset3.DesiredInstances.Int32 == newPrebuildCount
	}, testutil.WaitShort, testutil.IntervalFast)

	// gracefully stop the reconciliation loop
	reconciler.Stop(ctx, nil)
}

func TestFailedBuildBackoff(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}
	ctx := testutil.Context(t, testutil.WaitSuperLong)

	// Setup.
	clock := quartz.NewMock(t)
	backoffInterval := time.Minute
	cfg := codersdk.PrebuildsConfig{
		// Given: explicitly defined backoff configuration to validate timings.
		ReconciliationBackoffLookback: serpent.Duration(muchEarlier * -10), // Has to be positive.
		ReconciliationBackoffInterval: serpent.Duration(backoffInterval),
		ReconciliationInterval:        serpent.Duration(time.Second),
	}
	logger := slogtest.Make(
		t, &slogtest.Options{IgnoreErrors: true},
	).Leveled(slog.LevelDebug)
	db, ps := dbtestutil.NewDB(t)
	reconciler := prebuilds.NewStoreReconciler(db, ps, cfg, logger, clock, prometheus.NewRegistry())

	// Given: an active template version with presets and prebuilds configured.
	const desiredInstances = 2
	userID := uuid.New()
	dbgen.User(t, db, database.User{
		ID: userID,
	})
	org, template := setupTestDBTemplate(t, db, userID, false)
	templateVersionID := setupTestDBTemplateVersion(ctx, t, clock, db, ps, org.ID, userID, template.ID)

	preset := setupTestDBPreset(t, db, templateVersionID, desiredInstances, "test")
	for range desiredInstances {
		_ = setupTestDBPrebuild(t, clock, db, ps, database.WorkspaceTransitionStart, database.ProvisionerJobStatusFailed, org.ID, preset, template.ID, templateVersionID)
	}

	// When: determining what actions to take next, backoff is calculated because the prebuild is in a failed state.
	snapshot, err := reconciler.SnapshotState(ctx, db)
	require.NoError(t, err)
	require.Len(t, snapshot.Presets, 1)
	presetState, err := snapshot.FilterByPreset(preset.ID)
	require.NoError(t, err)
	state := presetState.CalculateState()
	actions, err := reconciler.CalculateActions(ctx, *presetState)
	require.NoError(t, err)

	// Then: the backoff time is in the future, no prebuilds are running, and we won't create any new prebuilds.
	require.EqualValues(t, 0, state.Actual)
	require.EqualValues(t, 0, actions.Create)
	require.EqualValues(t, desiredInstances, state.Desired)
	require.True(t, clock.Now().Before(actions.BackoffUntil))

	// Then: the backoff time is as expected based on the number of failed builds.
	require.NotNil(t, presetState.Backoff)
	require.EqualValues(t, desiredInstances, presetState.Backoff.NumFailed)
	require.EqualValues(t, backoffInterval*time.Duration(presetState.Backoff.NumFailed), clock.Until(actions.BackoffUntil).Truncate(backoffInterval))

	// When: advancing to the next tick which is still within the backoff time.
	clock.Advance(cfg.ReconciliationInterval.Value())

	// Then: the backoff interval will not have changed.
	snapshot, err = reconciler.SnapshotState(ctx, db)
	require.NoError(t, err)
	presetState, err = snapshot.FilterByPreset(preset.ID)
	require.NoError(t, err)
	newState := presetState.CalculateState()
	newActions, err := reconciler.CalculateActions(ctx, *presetState)
	require.NoError(t, err)
	require.EqualValues(t, 0, newState.Actual)
	require.EqualValues(t, 0, newActions.Create)
	require.EqualValues(t, desiredInstances, newState.Desired)
	require.EqualValues(t, actions.BackoffUntil, newActions.BackoffUntil)

	// When: advancing beyond the backoff time.
	clock.Advance(clock.Until(actions.BackoffUntil.Add(time.Second)))

	// Then: we will attempt to create a new prebuild.
	snapshot, err = reconciler.SnapshotState(ctx, db)
	require.NoError(t, err)
	presetState, err = snapshot.FilterByPreset(preset.ID)
	require.NoError(t, err)
	state = presetState.CalculateState()
	actions, err = reconciler.CalculateActions(ctx, *presetState)
	require.NoError(t, err)
	require.EqualValues(t, 0, state.Actual)
	require.EqualValues(t, desiredInstances, state.Desired)
	require.EqualValues(t, desiredInstances, actions.Create)

	// When: the desired number of new prebuild are provisioned, but one fails again.
	for i := 0; i < desiredInstances; i++ {
		status := database.ProvisionerJobStatusFailed
		if i == 1 {
			status = database.ProvisionerJobStatusSucceeded
		}
		_ = setupTestDBPrebuild(t, clock, db, ps, database.WorkspaceTransitionStart, status, org.ID, preset, template.ID, templateVersionID)
	}

	// Then: the backoff time is roughly equal to two backoff intervals, since another build has failed.
	snapshot, err = reconciler.SnapshotState(ctx, db)
	require.NoError(t, err)
	presetState, err = snapshot.FilterByPreset(preset.ID)
	require.NoError(t, err)
	state = presetState.CalculateState()
	actions, err = reconciler.CalculateActions(ctx, *presetState)
	require.NoError(t, err)
	require.EqualValues(t, 1, state.Actual)
	require.EqualValues(t, desiredInstances, state.Desired)
	require.EqualValues(t, 0, actions.Create)
	require.EqualValues(t, 3, presetState.Backoff.NumFailed)
	require.EqualValues(t, backoffInterval*time.Duration(presetState.Backoff.NumFailed), clock.Until(actions.BackoffUntil).Truncate(backoffInterval))
}

func TestReconciliationLock(t *testing.T) {
	t.Parallel()

	if !dbtestutil.WillUsePostgres() {
		t.Skip("This test requires postgres")
	}

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug)
	db, ps := dbtestutil.NewDB(t)

	wg := sync.WaitGroup{}
	mutex := sync.Mutex{}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reconciler := prebuilds.NewStoreReconciler(
				db,
				ps,
				codersdk.PrebuildsConfig{},
				slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}).Leveled(slog.LevelDebug),
				quartz.NewMock(t),
				prometheus.NewRegistry())
			reconciler.WithReconciliationLock(ctx, logger, func(_ context.Context, _ database.Store) error {
				lockObtained := mutex.TryLock()
				// As long as the postgres lock is held, this mutex should always be unlocked when we get here.
				// If this mutex is ever locked at this point, then that means that the postgres lock is not being held while we're
				// inside WithReconciliationLock, which is meant to hold the lock.
				require.True(t, lockObtained)
				// Sleep a bit to give reconcilers more time to contend for the lock
				time.Sleep(time.Second)
				defer mutex.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()
}

// nolint:revive // It's a control flag, but this is a test.
func setupTestDBTemplate(
	t *testing.T,
	db database.Store,
	userID uuid.UUID,
	templateDeleted bool,
) (
	database.Organization,
	database.Template,
) {
	t.Helper()
	org := dbgen.Organization(t, db, database.Organization{})

	template := dbgen.Template(t, db, database.Template{
		CreatedBy:      userID,
		OrganizationID: org.ID,
		CreatedAt:      time.Now().Add(muchEarlier),
	})
	if templateDeleted {
		ctx := testutil.Context(t, testutil.WaitShort)
		require.NoError(t, db.UpdateTemplateDeletedByID(ctx, database.UpdateTemplateDeletedByIDParams{
			ID:      template.ID,
			Deleted: true,
		}))
	}
	return org, template
}

const (
	earlier     = -time.Hour
	muchEarlier = -time.Hour * 2
)

func setupTestDBTemplateVersion(
	ctx context.Context,
	t *testing.T,
	clock quartz.Clock,
	db database.Store,
	ps pubsub.Pubsub,
	orgID uuid.UUID,
	userID uuid.UUID,
	templateID uuid.UUID,
) uuid.UUID {
	t.Helper()
	templateVersionJob := dbgen.ProvisionerJob(t, db, ps, database.ProvisionerJob{
		CreatedAt:      clock.Now().Add(muchEarlier),
		CompletedAt:    sql.NullTime{Time: clock.Now().Add(earlier), Valid: true},
		OrganizationID: orgID,
		InitiatorID:    userID,
	})
	templateVersion := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		TemplateID:     uuid.NullUUID{UUID: templateID, Valid: true},
		OrganizationID: orgID,
		CreatedBy:      userID,
		JobID:          templateVersionJob.ID,
		CreatedAt:      time.Now().Add(muchEarlier),
	})
	require.NoError(t, db.UpdateTemplateActiveVersionByID(ctx, database.UpdateTemplateActiveVersionByIDParams{
		ID:              templateID,
		ActiveVersionID: templateVersion.ID,
	}))
	// Make sure immutable params don't break prebuilt workspace deletion logic
	dbgen.TemplateVersionParameter(t, db, database.TemplateVersionParameter{
		TemplateVersionID: templateVersion.ID,
		Name:              "test",
		Description:       "required & immutable param",
		Type:              "string",
		DefaultValue:      "",
		Required:          true,
		Mutable:           false,
	})
	return templateVersion.ID
}

func setupTestDBPreset(
	t *testing.T,
	db database.Store,
	templateVersionID uuid.UUID,
	desiredInstances int32,
	presetName string,
) database.TemplateVersionPreset {
	t.Helper()
	preset := dbgen.Preset(t, db, database.InsertPresetParams{
		TemplateVersionID: templateVersionID,
		Name:              presetName,
		DesiredInstances: sql.NullInt32{
			Valid: true,
			Int32: desiredInstances,
		},
	})
	dbgen.PresetParameter(t, db, database.InsertPresetParametersParams{
		TemplateVersionPresetID: preset.ID,
		Names:                   []string{"test"},
		Values:                  []string{"test"},
	})
	return preset
}

func setupTestDBPrebuild(
	t *testing.T,
	clock quartz.Clock,
	db database.Store,
	ps pubsub.Pubsub,
	transition database.WorkspaceTransition,
	prebuildStatus database.ProvisionerJobStatus,
	orgID uuid.UUID,
	preset database.TemplateVersionPreset,
	templateID uuid.UUID,
	templateVersionID uuid.UUID,
) database.WorkspaceTable {
	t.Helper()
	return setupTestDBWorkspace(t, clock, db, ps, transition, prebuildStatus, orgID, preset, templateID, templateVersionID, agplprebuilds.SystemUserID, agplprebuilds.SystemUserID)
}

func setupTestDBWorkspace(
	t *testing.T,
	clock quartz.Clock,
	db database.Store,
	ps pubsub.Pubsub,
	transition database.WorkspaceTransition,
	prebuildStatus database.ProvisionerJobStatus,
	orgID uuid.UUID,
	preset database.TemplateVersionPreset,
	templateID uuid.UUID,
	templateVersionID uuid.UUID,
	initiatorID uuid.UUID,
	ownerID uuid.UUID,
) database.WorkspaceTable {
	t.Helper()
	cancelledAt := sql.NullTime{}
	completedAt := sql.NullTime{}

	startedAt := sql.NullTime{}
	if prebuildStatus != database.ProvisionerJobStatusPending {
		startedAt = sql.NullTime{Time: clock.Now().Add(muchEarlier), Valid: true}
	}

	buildError := sql.NullString{}
	if prebuildStatus == database.ProvisionerJobStatusFailed {
		completedAt = sql.NullTime{Time: clock.Now().Add(earlier), Valid: true}
		buildError = sql.NullString{String: "build failed", Valid: true}
	}

	switch prebuildStatus {
	case database.ProvisionerJobStatusCanceling:
		cancelledAt = sql.NullTime{Time: clock.Now().Add(earlier), Valid: true}
	case database.ProvisionerJobStatusCanceled:
		completedAt = sql.NullTime{Time: clock.Now().Add(earlier), Valid: true}
		cancelledAt = sql.NullTime{Time: clock.Now().Add(earlier), Valid: true}
	case database.ProvisionerJobStatusSucceeded:
		completedAt = sql.NullTime{Time: clock.Now().Add(earlier), Valid: true}
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
		CreatedAt:      clock.Now().Add(muchEarlier),
		StartedAt:      startedAt,
		CompletedAt:    completedAt,
		CanceledAt:     cancelledAt,
		OrganizationID: orgID,
		Error:          buildError,
	})
	workspaceBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		WorkspaceID:             workspace.ID,
		InitiatorID:             initiatorID,
		TemplateVersionID:       templateVersionID,
		JobID:                   job.ID,
		TemplateVersionPresetID: uuid.NullUUID{UUID: preset.ID, Valid: true},
		Transition:              transition,
		CreatedAt:               clock.Now(),
	})
	dbgen.WorkspaceBuildParameters(t, db, []database.WorkspaceBuildParameter{
		{
			WorkspaceBuildID: workspaceBuild.ID,
			Name:             "test",
			Value:            "test",
		},
	})

	return workspace
}

// nolint:revive // It's a control flag, but this is a test.
func setupTestDBWorkspaceAgent(t *testing.T, db database.Store, workspaceID uuid.UUID, eligible bool) database.WorkspaceAgent {
	build, err := db.GetLatestWorkspaceBuildByWorkspaceID(t.Context(), workspaceID)
	require.NoError(t, err)

	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{JobID: build.JobID})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID: res.ID,
	})

	// A prebuilt workspace is considered eligible when its agent is in a "ready" lifecycle state.
	// i.e. connected to the control plane and all startup scripts have run.
	if eligible {
		require.NoError(t, db.UpdateWorkspaceAgentLifecycleStateByID(t.Context(), database.UpdateWorkspaceAgentLifecycleStateByIDParams{
			ID:             agent.ID,
			LifecycleState: database.WorkspaceAgentLifecycleStateReady,
			StartedAt:      sql.NullTime{Time: dbtime.Now().Add(-time.Minute), Valid: true},
			ReadyAt:        sql.NullTime{Time: dbtime.Now(), Valid: true},
		}))
	}

	return agent
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

func allJobStatusesExcept(except ...database.ProvisionerJobStatus) []database.ProvisionerJobStatus {
	return slice.Filter(except, func(status database.ProvisionerJobStatus) bool {
		return !slice.Contains(allJobStatuses, status)
	})
}
